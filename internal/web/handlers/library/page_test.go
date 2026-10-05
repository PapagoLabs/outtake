// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/handlers/library/mocks"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

type pmsStub struct {
	// client is a Plex client bound to the stub.
	client *plex.Client
	// server is the Plex server the stub answers as.
	server plex.Server
}
type pageAnswer struct {
	status int
	header http.Header
	body   string
}

// pmsStub is a loopback server standing in for a Plex Media Server.

// startPMS starts a loopback server answering with the given handler.
//
// Parameters:
//   - t: The test the server belongs to.
//   - handler: Handler answering every request.
//
// Returns:
//   - stub: The stub, closed when the test ends.
func startPMS(t *testing.T, handler http.HandlerFunc) *pmsStub {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	host, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	require.NoError(t, err)

	portNumber, err := strconv.Atoi(port)
	require.NoError(t, err)

	return &pmsStub{
		client: plex.NewClient(plex.ClientConfig{
			Product: "outtake-test",
			Token:   "page-token",
			Timeout: 5 * time.Second,
		}),
		server: plex.Server{
			Name:    "loopback",
			Address: host,
			Port:    portNumber,
			Scheme:  "http",
			Token:   "page-token",
		},
	}
}

// sourceInfo builds a source description the media pages read.
//
// Parameters:
//   - duration: How long the probed source runs.
//
// Returns:
//   - info: The source description.
func sourceInfo(duration time.Duration) library.SourceInfo {
	return library.SourceInfo{
		Path:     "/media/movie.mkv",
		Duration: duration,
		HDR:      true,
		Quality:  string(clip.ClipQualityHigh),
		AudioStreams: []library.AudioStream{
			{Index: 0, Codec: "eac3", Language: "eng", Channels: 6, Layout: "5.1"},
			{Index: 1, Codec: "aac", Language: "jpn", Channels: 2, Layout: "stereo"},
		},
	}
}

// pageHandler wires a pages handler around the given collaborators.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - auth: Plex authentication the pages read.
//   - sources: Source describer the media pages read.
//
// Returns:
//   - handler: The handler under test.
//   - db: The database the pages read through.
func pageHandler(
	t *testing.T,
	auth PlexAuth,
	sources *mocks.MockMediaDescriber,
) (*Handler, *database.DB) {
	t.Helper()

	db, err := database.New(t.TempDir() + "/pages.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return New(
		queueForTest(t),
		db,
		auth,
		&config.Config{MaxClipDur: 15 * time.Minute},
		sources,
	), db
}

// offlineAuth returns a Plex authentication with nothing bound and no sessions.
//
// Parameters:
//   - t: The test the authentication belongs to.
//
// Returns:
//   - auth: The authentication under test.
func offlineAuth(t *testing.T) *mocks.MockPlexAuth {
	t.Helper()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Client().Return(nil, plex.EmptyServer(), false).Maybe()
	auth.EXPECT().Sessions().Return(nil).Maybe()
	auth.EXPECT().Selected().Return(plex.EmptyServer(), false).Maybe()

	return auth
}

// newMockAuth builds a Plex authentication with nothing wired up.
//
// Parameters:
//   - t: The test the authentication belongs to.
//
// Returns:
//   - auth: The authentication under test.
//
// silentSources returns a describer that resolves nothing.
//
// Parameters:
//   - t: The test the describer belongs to.
//
// Returns:
//   - sources: The describer under test.
func silentSources(t *testing.T) *mocks.MockMediaDescriber {
	t.Helper()

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().
		Describe(mock.Anything, mock.Anything).
		Return(library.SourceInfo{}).
		Maybe()

	return sources
}

// pageAnswer is what one served page request produced.

// serve issues one GET request against a mounted app.
//
// Parameters:
//   - t: The test the request belongs to.
//   - app: The app to serve.
//   - target: Request target, including any query string.
//   - htmx: Whether to mark the request as coming from HTMX.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func serve(
	t *testing.T,
	app *fiber.App,
	target string,
	htmx bool,
) pageAnswer {
	t.Helper()

	return serveMethod(t, app, http.MethodGet, target, htmx, "", "")
}

// serveMethod issues one request against a mounted app.
//
// Parameters:
//   - t: The test the request belongs to.
//   - app: The app to serve.
//   - method: HTTP method to issue.
//   - target: Request target, including any query string.
//   - htmx: Whether to mark the request as coming from HTMX.
//   - hxTarget: HX-Target header value, empty to omit the header.
//   - form: Form body, empty for none.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func serveMethod(
	t *testing.T,
	app *fiber.App,
	method, target string,
	htmx bool,
	hxTarget string,
	form string,
) pageAnswer {
	t.Helper()

	var req *http.Request

	if form == "" {
		req = httptest.NewRequestWithContext(t.Context(), method, target, nil)
	} else {
		req = httptest.NewRequestWithContext(
			t.Context(), method, target, strings.NewReader(form),
		)
		req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")
	}

	if htmx {
		req.Header.Set(routes.HeaderHXRequest, "true")
	}

	if hxTarget != "" {
		req.Header.Set(routes.HeaderHXTarget, hxTarget)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return pageAnswer{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

// assertBodyContains reports whether a rendered page contains a fragment. A full
// page is too large to print in a failure, so the fragment is named instead.
//
// Parameters:
//   - t: The test the page belongs to.
//   - body: The rendered page.
//   - want: Fragment the page has to contain.
//   - reason: What the fragment proves.
func assertBodyContains(t *testing.T, body, want, reason string) {
	t.Helper()

	assert.Contains(t, body, want, "%s", reason)
}

// assertBodyOmits reports whether a rendered page leaves a fragment out.
//
// Parameters:
//   - t: The test the page belongs to.
//   - body: The rendered page.
//   - unwanted: Fragment the page must not contain.
//   - reason: What the absence proves.
func assertBodyOmits(t *testing.T, body, unwanted, reason string) {
	t.Helper()

	assert.NotContains(t, body, unwanted, "%s (found %q)", reason, unwanted)
}
