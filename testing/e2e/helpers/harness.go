// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

// Package helpers is the shared end-to-end harness.
//
// Every functional area lives in its own directory so that it compiles to its
// own test binary with its own Ginkgo suite. Go cannot share a `_test.go` file
// across packages, so the harness that every area needs — the environment
// loader, the suite lifecycle, the HTTP helpers, and the test video generator —
// has to be an ordinary importable package rather than a test file.
package helpers

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/app"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// App is the wired application one e2e suite package drives.
type App struct {
	// Application is the composition root under test.
	Application *app.App
	// BaseURL is the origin every request URL is built from.
	BaseURL string
	// Listener holds the address the app is configured to listen on. The
	// composition root exposes no way to serve on an existing listener, so the
	// harness pins the address here and drives requests through Application.Test.
	Listener net.Listener
	// Dir is the temporary directory holding the database and the storage tree.
	Dir string
	// MediaDir is where the generated test video is written.
	MediaDir string
	// TestVideo is the generated test video, empty when ffmpeg is unavailable.
	TestVideo string
	// FFmpegOK reports whether ffmpeg and ffprobe are both on PATH.
	FFmpegOK bool
	// PlexOK reports whether usable Plex credentials were resolved.
	PlexOK bool
	// PlexReason explains why the Plex-dependent specs are skipped.
	PlexReason string
	// Server is the Plex server PLEX_SERVER_URL named.
	Server plex.Server
	// Token is the resolved Plex access token.
	Token string
}

// The Plex credentials and the runtime shape every area's suite shares.
const (
	// Product is the Plex product name the suite identifies itself under.
	Product = "outtake-e2e"

	// ClientID is the Plex client identifier the suite sends.
	ClientID = "outtake-e2e-test"

	// PlexTVBaseURL is the plex.tv API the account-scoped calls are made against.
	PlexTVBaseURL = "https://plex.tv"

	// Environment is the configuration environment the app under test runs
	// under. The web layer's authentication guard and the library media path
	// resolver both branch on it, so every spec depends on it being e2e: with it
	// the pages are reachable without a session, and a media id may be a local
	// file path rather than a Plex rating key.
	Environment = "e2e"
)

// The suite's fixed runtime settings.
const (
	// tempPrefix is the temporary directory prefix one suite package works in.
	tempPrefix = "outtake-e2e-"

	// listenNetwork and listenAddr reserve a loopback port for the suite.
	listenNetwork = "tcp"
	listenAddr    = "127.0.0.1:0"

	// sessionPollSeconds is the Plex session poll interval.
	sessionPollSeconds = 10

	// numWorkers is the clip worker count.
	numWorkers = 2

	// maxConcurrentPreviews is the preview render concurrency. It has to be
	// positive: the preview gate admits nothing when it is left at zero.
	maxConcurrentPreviews = 2

	// maxClipDuration is the longest clip the suite asks for.
	maxClipDuration = 600 * time.Second

	// dirPerms is the mode the suite's media directory is created with.
	dirPerms = 0o755
)

// The HTTP paths the suite talks to.
const (
	// HealthPath is the health endpoint, which is also the safe request the
	// suite mints CSRF tokens from.
	HealthPath = "/api/healthz"

	// ClipsPath is the clip collection API path.
	ClipsPath = "/api/clips"

	// clipStatusFormat is the clip status API path for one job.
	clipStatusFormat = "/api/clips/%s/status"

	// clipDownloadFormat is the clip download API path for one job.
	clipDownloadFormat = "/api/clips/%s/download"
)

// ClipStatusPath returns the status endpoint for one clip job.
//
// Parameters:
//   - clipID: Identifier of the clip job.
//
// Returns:
//   - path: Path of the clip's status endpoint.
func ClipStatusPath(clipID string) string {
	return fmt.Sprintf(clipStatusFormat, clipID)
}

// ClipDownloadPath returns the download endpoint for one clip job.
//
// Parameters:
//   - clipID: Identifier of the clip job.
//
// Returns:
//   - path: Path of the clip's download endpoint.
func ClipDownloadPath(clipID string) string {
	return fmt.Sprintf(clipDownloadFormat, clipID)
}

// New returns an unwired harness for one suite package. BeforeSuite fills it in.
func New() *App {
	return &App{}
}

// BeforeSuite loads the e2e environment, reserves a listen address, wires the
// application, and generates the test video when ffmpeg is available.
//
// Every suite package calls this from its own BeforeSuite, because a compiled
// test binary carries exactly one Ginkgo suite and therefore its own suite
// lifecycle.
func (a *App) BeforeSuite() {
	ginkgo.GinkgoHelper()

	loaded, err := LoadEnv()
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "load the e2e environment")

	loaded.Report()

	a.reserve()
	a.FFmpegOK = OnPath(ffmpegBin, ffprobeBin)
	a.resolvePlex()

	cfg := a.config()
	a.BaseURL = "http://" + cfg.ListenAddr

	logging.InitFromConfig(cfg)

	application, err := app.New(cfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "build the e2e application")

	a.Application = application

	if a.FFmpegOK {
		video, videoErr := GenerateTestVideo(context.Background(), a.MediaDir)
		gomega.Expect(videoErr).NotTo(
			gomega.HaveOccurred(),
			"generate the e2e test video",
		)

		a.TestVideo = video
	}
}

// AfterSuite closes the application and removes the temporary directory.
func (a *App) AfterSuite() {
	ginkgo.GinkgoHelper()

	if a.Application != nil {
		a.Application.Close()
	}

	if a.Listener != nil {
		_ = a.Listener.Close()
	}

	if a.Dir != "" {
		_ = os.RemoveAll(a.Dir)
	}
}

// reserve creates the suite's temporary tree and binds the loopback listener
// that pins the application under test to one known address.
func (a *App) reserve() {
	ginkgo.GinkgoHelper()

	dir, err := os.MkdirTemp("", tempPrefix)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "create the e2e working directory")

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), listenNetwork, listenAddr)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "reserve the e2e listen address")

	a.Dir = dir
	a.MediaDir = filepath.Join(dir, "media")
	a.Listener = listener
}

// config builds the configuration the application under test is wired from. The
// listen address comes from the reserved listener, so the configuration and the
// advertised base URL are the same value by construction.
func (a *App) config() *config.Config {
	return &config.Config{
		ListenAddr:            a.Listener.Addr().String(),
		DatabasePath:          filepath.Join(a.Dir, "outtake.db"),
		StoragePath:           filepath.Join(a.Dir, "storage"),
		FFmpegPath:            ResolveBinary(ffmpegBin),
		FFprobePath:           ResolveBinary(ffprobeBin),
		LogLevel:              "error",
		Env:                   Environment,
		SessionPoll:           sessionPollSeconds * time.Second,
		NumWorkers:            numWorkers,
		MaxConcurrentPreviews: maxConcurrentPreviews,
		MaxClipDur:            maxClipDuration,
		CropBlackBars:         false,
		PlexServerURL:         os.Getenv(ServerURLEnv),
		PlexToken:             a.Token,
		PlexClientID:          ClientID,
	}
}

// resolvePlex records whether usable Plex credentials were supplied and builds
// the server they name. Missing credentials are not an error: they only mean
// the Plex-dependent specs skip.
func (a *App) resolvePlex() {
	ginkgo.GinkgoHelper()

	token := strings.TrimSpace(os.Getenv(TokenEnv))
	serverURL := strings.TrimSpace(os.Getenv(ServerURLEnv))

	var missing []string
	if token == "" {
		missing = append(missing, TokenEnv)
	}

	if serverURL == "" {
		missing = append(missing, ServerURLEnv)
	}

	if len(missing) > 0 {
		a.PlexReason = fmt.Sprintf(
			"Plex credentials unavailable: %s is not set; export it or add it to %s",
			strings.Join(missing, " and "),
			EnvLocation(),
		)

		return
	}

	server, ok := plex.ServerFromURL(serverURL, token)
	if !ok {
		a.PlexReason = fmt.Sprintf(
			"Plex credentials unavailable: %s is not a usable Plex server URL",
			ServerURLEnv,
		)

		return
	}

	a.PlexOK = true
	a.Token = token
	a.Server = server
}

// SkipWithoutPlex skips the calling spec when Plex credentials were not
// resolved, carrying the reason the suite computed for it.
//
// Parameters:
//   - none.
//
// Returns:
//   - nothing; the calling spec is abandoned when the credentials are missing.
func (a *App) SkipWithoutPlex() {
	ginkgo.GinkgoHelper()

	if a.PlexOK {
		return
	}

	ginkgo.Skip(a.PlexReason)
}

// SkipWithoutFFmpeg skips the calling spec when ffmpeg is not on PATH.
//
// Returns:
//   - nothing; the calling spec is abandoned when the binaries are missing.
func (a *App) SkipWithoutFFmpeg() {
	ginkgo.GinkgoHelper()

	if a.FFmpegOK {
		return
	}

	ginkgo.Skip("ffmpeg and ffprobe are not on PATH, so no clip or preview can be rendered")
}

// PlexClient returns a Plex client branded for the suite.
//
// Parameters:
//   - token: Plex access token the client authenticates with.
//   - baseURL: API origin the client talks to.
//   - timeout: Request timeout.
//
// Returns:
//   - client: The branded Plex client.
func PlexClient(token, baseURL string, timeout time.Duration) *plex.Client {
	return plex.NewClient(plex.ClientConfig{
		Product:  Product,
		ClientID: ClientID,
		Token:    token,
		Timeout:  timeout,
		BaseURL:  baseURL,
	})
}
