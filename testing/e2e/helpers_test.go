// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/app"
	"github.com/PapagoLabs/outtake/internal/config"
	clipapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/clip"
	mediaapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/media"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
)

type e2eApp struct {
	app       *app.App
	ffmpegOK  bool
	plexOK    bool
	mediaDir  string
	testVideo string
}

const (
	testBaseURL    = "http://127.0.0.1:8080"
	outtakeE2E     = "outtake-e2e"
	outtakeE2ETest = "outtake-e2e-test"
)

var (
	testApp  *e2eApp
	testDir  string
	ffmpegOK bool
)

var _ = BeforeSuite(func() {
	loadEnv()

	dir, err := os.MkdirTemp("", "outtake-e2e-")
	Expect(err).NotTo(HaveOccurred())

	testDir = dir

	dbPath := filepath.Join(dir, "outtake.db")
	storagePath := filepath.Join(dir, "storage")
	mediaDir := filepath.Join(dir, "media")

	ffmpegOK = false
	_, err = exec.LookPath("ffmpeg")
	if err == nil {
		ffmpegOK = true
	}

	cfg := &config.Config{
		ListenAddr:     "127.0.0.1:0",
		DatabasePath:   dbPath,
		StoragePath:    storagePath,
		FFmpegPath:     resolveFFmpegPath(),
		FFprobePath:    resolveFFprobePath(),
		LogLevel:       "error",
		Env:            "e2e",
		SessionPollSec: 10,
		NumWorkers:     2,
		MaxClipDurSec:  600,
		CropBlackBars:  false,
		PlexServerURL:  os.Getenv("PLEX_SERVER_URL"),
		PlexToken:      os.Getenv("PLEX_TOKEN"),
		PlexClientID:   outtakeE2ETest,
	}

	appInstance, err := app.New(cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create e2e app")

	testApp = &e2eApp{
		app:       appInstance,
		ffmpegOK:  ffmpegOK,
		plexOK:    os.Getenv("PLEX_TOKEN") != "" && os.Getenv("PLEX_SERVER_URL") != "",
		mediaDir:  mediaDir,
		testVideo: "",
	}

	if ffmpegOK {
		testApp.testVideo = generateTestVideo(mediaDir)
	}
})

var _ = AfterSuite(func() {
	if testApp != nil {
		testApp.app.Close()
	}
	if testDir != "" {
		_ = os.RemoveAll(testDir)
	}
})

// resolveFFmpegPath returns the ffmpeg binary path for e2e tests.
//
// Returns:
//   - path: Path to ffmpeg.
func resolveFFmpegPath() string {
	path, err := exec.LookPath("ffmpeg")
	if err == nil {
		return path
	}

	return "ffmpeg"
}

// resolveFFprobePath returns the ffprobe binary path for e2e tests.
//
// Returns:
//   - path: Path to ffprobe.
func resolveFFprobePath() string {
	path, err := exec.LookPath("ffprobe")
	if err == nil {
		return path
	}

	return "ffprobe"
}

// generateTestVideo writes a short synthetic test video under mediaDir.
//
// Parameters:
//   - mediaDir: Directory for the generated file.
//
// Returns:
//   - path: Absolute path to the generated video.
func generateTestVideo(mediaDir string) string {
	Expect(os.MkdirAll(mediaDir, 0o755)).To(Succeed())

	testVideo := filepath.Join(mediaDir, "test-video.mp4")

	cmd := exec.CommandContext(
		context.Background(),
		resolveFFmpegPath(),
		"-f", "lavfi",
		"-i", "testsrc=duration=1:size=160x120:rate=24",
		"-c:v", "libx264",
		"-t", "1",
		"-pix_fmt", "yuv420p",
		"-y",
		testVideo,
	)
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "failed to generate test video: %s", string(output))

	return testVideo
}

// testMediaID returns a stable synthetic media ID for absPath.
//
// Parameters:
//   - absPath: Absolute media file path.
//
// Returns:
//   - id: Media identifier.
func testMediaID(absPath string) string {
	return absPath
}

// doRequest performs an HTTP request against the e2e server.
//
// Parameters:
//   - method: HTTP method.
//   - path: Request path.
//
// Returns:
//   - resp: HTTP response.
func doRequest(method, path string) *http.Response {
	GinkgoHelper()

	req, err := http.NewRequestWithContext(context.Background(), method, testBaseURL+path, nil)
	Expect(err).NotTo(HaveOccurred())

	resp, err := testApp.app.Test(req)
	Expect(err).NotTo(HaveOccurred())

	return resp
}

// doJSONRequest posts JSON body to the e2e create-clip endpoint.
//
// Parameters:
//   - body: Value encoded as JSON.
//
// Returns:
//   - resp: HTTP response.
func doJSONRequest(body any) *http.Response {
	GinkgoHelper()

	jsonBody, err := json.Marshal(body)
	Expect(err).NotTo(HaveOccurred())

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		testBaseURL+"/api/clips",
		strings.NewReader(string(jsonBody)),
	)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")

	resp, err := testApp.app.Test(req)
	Expect(err).NotTo(HaveOccurred())

	return resp
}

// decodeClipResponse decodes a clip JSON response body.
//
// Parameters:
//   - body: Response body bytes.
//
// Returns:
//   - clip: Decoded clip payload.
func decodeClipResponse(body []byte) clipapi.ClipResponse {
	GinkgoHelper()

	var resp clipapi.ClipResponse

	Expect(json.Unmarshal(body, &resp)).To(Succeed())

	return resp
}

// decodeListResponse decodes a media list JSON response body.
//
// Parameters:
//   - body: Response body bytes.
//
// Returns:
//   - list: Decoded list payload.
func decodeListResponse(body []byte) mediaapi.MediaListResponse {
	GinkgoHelper()

	var resp mediaapi.MediaListResponse

	Expect(json.Unmarshal(body, &resp)).To(Succeed())

	return resp
}

// decodeErrorResponse decodes an error JSON response body.
//
// Parameters:
//   - body: Response body bytes.
//
// Returns:
//   - payload: Decoded error payload.
func decodeErrorResponse(body []byte) respond.ErrorResponse {
	GinkgoHelper()

	var resp respond.ErrorResponse

	Expect(json.Unmarshal(body, &resp)).To(Succeed())

	return resp
}

// waitForClipStatus polls until the clip reaches expectedStatus or times out.
//
// Parameters:
//   - jobID: Clip job ID.
//   - expectedStatus: Desired status string.
//   - timeout: Maximum time to wait.
func waitForClipStatus(jobID, expectedStatus string, timeout time.Duration) {
	GinkgoHelper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp := doRequest(http.MethodGet, "/api/clips/"+jobID+"/status")
		body, err := io.ReadAll(resp.Body)
		closeBody(resp)

		if err != nil {
			time.Sleep(100 * time.Millisecond)

			continue
		}

		var clip clipapi.ClipResponse

		jsonErr := json.Unmarshal(body, &clip)
		if jsonErr != nil {
			time.Sleep(100 * time.Millisecond)

			continue
		}

		if clip.Status == expectedStatus {
			return
		}

		if clip.Status == "failed" {
			Fail("clip job failed: " + clip.Error)
		}

		time.Sleep(100 * time.Millisecond)
	}

	Fail("timeout waiting for clip status")
}
