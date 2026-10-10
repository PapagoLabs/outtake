// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/handlers/server/mocks"
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
	// cookies carry the session the response used.
	cookies []*http.Cookie
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

// pageHandler wires a pages handler around the given collaborators.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - auth: Plex authentication the pages read.
//   - sources: Source describer the media pages read.
//
// Returns:
//   - handler: The handler under test.
func pageHandler(
	t *testing.T,
	auth PlexAuth,
	sources *mocks.MockMediaDescriber,
) *Handler {
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
	)
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
	auth.EXPECT().TokenRejected().Return(false).Maybe()
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

	return serveMethod(t, app, http.MethodGet, target, htmx, "")
}

// serveMethod issues one request against a mounted app.
//
// Parameters:
//   - t: The test the request belongs to.
//   - app: The app to serve.
//   - method: HTTP method to issue.
//   - target: Request target, including any query string.
//   - htmx: Whether to mark the request as coming from HTMX.
//   - form: Form body, empty for none.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func serveMethod(
	t *testing.T,
	app *fiber.App,
	method, target string,
	htmx bool,
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

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return pageAnswer{
		status:  resp.StatusCode,
		header:  resp.Header,
		body:    string(body),
		cookies: resp.Cookies(),
	}
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

// sessionMiddleware builds an in-memory session middleware for a test app.
//
// Parameters:
//   - t: The test the middleware belongs to.
//
// Returns:
//   - middleware: The middleware to install on the app.
func sessionMiddleware(t *testing.T) fiber.Handler {
	t.Helper()

	middleware, store := session.NewWithStore(session.Config{})
	require.NotNil(t, store)

	return middleware
}

// seedTokenFor stores a Plex token on the session of a throwaway request so the
// browser that follows it carries one.
//
// Parameters:
//   - t: The test the session belongs to.
//   - app: The app, which already carries the session middleware.
//   - token: Token to store, empty to store none.
//
// Returns:
//   - cookies: Session cookie to send with the request that follows.
func seedTokenFor(
	t *testing.T,
	app *fiber.App,
	token string,
) []*http.Cookie {
	t.Helper()

	app.Get("/session/seed", func(ctx fiber.Ctx) error {
		if token != "" {
			identity.SetToken(session.FromContext(ctx), token)
		}

		return ctx.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/session/seed", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	return resp.Cookies()
}

// requestWithCookies builds one request carrying the given cookies.
//
// Parameters:
//   - t: The test the request belongs to.
//   - method: HTTP method to issue.
//   - target: Request target, including any query string.
//   - form: Form body, empty for none.
//   - cookies: Cookies to send.
//
// Returns:
//   - req: The request to hand to app.Test.
func requestWithCookies(
	t *testing.T,
	method, target string,
	form string,
	cookies []*http.Cookie,
) *http.Request {
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

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	return req
}

// serveWithCookies issues one GET request carrying the given cookies.
//
// Parameters:
//   - t: The test the request belongs to.
//   - app: The app to serve.
//   - cookies: Cookies to send.
//   - target: Request target, including any query string.
//   - form: Form body, empty for none.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func serveWithCookies(
	t *testing.T,
	app *fiber.App,
	cookies []*http.Cookie,
	target string,
	form string,
) pageAnswer {
	t.Helper()

	req := requestWithCookies(t, http.MethodGet, target, form, cookies)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return pageAnswer{
		status:  resp.StatusCode,
		header:  resp.Header,
		body:    string(body),
		cookies: resp.Cookies(),
	}
}
