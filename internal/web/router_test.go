// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// routerRoute is one row of the route table New is expected to register.
type routerRoute struct {
	// method is the HTTP method the route answers.
	method string
	// path is the registered route pattern.
	path string
	// guarded is true when the auth guard sits in front of the handler.
	guarded bool
	// sample is a concrete request path with the parameters filled in.
	sample string
}

// answer is everything a router test observes about one response.
type answer struct {
	// status is the response status code.
	status int
	// location is the Location response header.
	location string
	// contentType is the Content-Type response header.
	contentType string
	// body is the response body, read to end.
	body string
	// cookies are the cookies the application set.
	cookies []*http.Cookie
}

// browser drives the router the way a browser would, carrying the cookies and
// the CSRF token the application handed out.
type browser struct {
	// app is the router under test.
	app *fiber.App
	// db is the store the owner and the selected server live in.
	db *database.DB
	// cookies are the cookies the application set.
	cookies []*http.Cookie
	// csrfToken is the token the CSRF cookie carries.
	csrfToken string
	// host is the Host header the browser sends.
	host string
}

const (
	// ownerToken is the Plex token the plex.tv stand-in maps to account 42.
	ownerToken = "plex-token"

	// otherToken is the Plex token the plex.tv stand-in maps to account 7.
	otherToken = "other-token"

	// routerHost is the Host header every browser request carries.
	routerHost = "localhost"
)

// wantRoutes is the complete route table New must register, in registration
// order, with each route classified as guarded or open.
var wantRoutes = []routerRoute{
	{method: http.MethodGet, path: "/login", sample: "/login"},
	{method: http.MethodGet, path: "/", guarded: true, sample: "/"},
	{
		method:  http.MethodGet,
		path:    "/dashboard/sessions",
		guarded: true,
		sample:  "/dashboard/sessions",
	},
	{method: http.MethodGet, path: "/media", guarded: true, sample: "/media"},
	{
		method:  http.MethodGet,
		path:    "/media/item/:id/playback",
		guarded: true,
		sample:  "/media/item/5/playback",
	},
	{
		method:  http.MethodGet,
		path:    "/media/item/:id/clips",
		guarded: true,
		sample:  "/media/item/5/clips",
	},
	{method: http.MethodGet, path: "/media/item/:id", guarded: true, sample: "/media/item/5"},
	{method: http.MethodGet, path: "/previews/:id", guarded: true, sample: "/previews/9"},
	{method: http.MethodGet, path: "/nav/libraries", guarded: true, sample: "/nav/libraries"},
	{method: http.MethodGet, path: "/thumbs", guarded: true, sample: "/thumbs"},
	{method: http.MethodGet, path: "/clips/new", guarded: true, sample: "/clips/new"},
	{method: http.MethodGet, path: "/clips/:id/file", guarded: true, sample: "/clips/7/file"},
	{method: http.MethodGet, path: "/clips/:id/row", guarded: true, sample: "/clips/7/row"},
	{method: http.MethodGet, path: routeClips, guarded: true, sample: routeClips},
	{method: http.MethodGet, path: "/servers", guarded: true, sample: "/servers"},
	{
		method:  http.MethodGet,
		path:    "/settings/appearance",
		guarded: true,
		sample:  "/settings/appearance",
	},
	{
		method:  http.MethodGet,
		path:    "/settings/profiles",
		guarded: true,
		sample:  "/settings/profiles",
	},
	{method: http.MethodPost, path: "/servers", guarded: true, sample: "/servers"},
	{method: http.MethodPost, path: "/servers/forget", guarded: true, sample: "/servers/forget"},
	{
		method:  http.MethodPost,
		path:    "/settings/profiles",
		guarded: true,
		sample:  "/settings/profiles",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/profiles/:id/default",
		guarded: true,
		sample:  "/settings/profiles/abc/default",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/profiles/:id/delete",
		guarded: true,
		sample:  "/settings/profiles/abc/delete",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/profiles/:id",
		guarded: true,
		sample:  "/settings/profiles/abc",
	},
	{method: http.MethodPost, path: "/api/clips", guarded: true, sample: "/api/clips"},
	{
		method:  http.MethodPost,
		path:    "/api/clips/preview",
		guarded: true,
		sample:  "/api/clips/preview",
	},
	{
		method:  http.MethodGet,
		path:    "/api/clips/preview/:id",
		guarded: true,
		sample:  "/api/clips/preview/abc",
	},
	{
		method:  http.MethodDelete,
		path:    "/api/clips/preview/:id",
		guarded: true,
		sample:  "/api/clips/preview/abc",
	},
	{
		method:  http.MethodPost,
		path:    "/api/clips/:id/update",
		guarded: true,
		sample:  "/api/clips/abc/update",
	},
	{
		method:  http.MethodPost,
		path:    "/api/clips/:id/cancel",
		guarded: true,
		sample:  "/api/clips/abc/cancel",
	},
	{method: http.MethodGet, path: "/api/clips", guarded: true, sample: "/api/clips"},
	{
		method:  http.MethodGet,
		path:    "/api/clips/:id/status",
		guarded: true,
		sample:  "/api/clips/abc/status",
	},
	{
		method:  http.MethodGet,
		path:    "/api/clips/:id/download",
		guarded: true,
		sample:  "/api/clips/abc/download",
	},
	{method: http.MethodDelete, path: "/api/clips/:id", guarded: true, sample: "/api/clips/abc"},
	{method: http.MethodGet, path: "/api/media/search", guarded: true, sample: "/api/media/search"},
	{method: http.MethodGet, path: "/api/sessions", guarded: true, sample: "/api/sessions"},
	{method: http.MethodPost, path: "/api/auth/login", sample: "/api/auth/login"},
	{method: http.MethodGet, path: "/api/auth/callback", sample: "/api/auth/callback"},
	{method: http.MethodGet, path: "/api/auth/status", sample: "/api/auth/status"},
	{method: http.MethodPost, path: "/api/auth/logout", sample: "/api/auth/logout"},
	{method: http.MethodGet, path: "/api/healthz", sample: "/api/healthz"},
}

func TestNewRegistersExactlyTheExpectedRouteTable(t *testing.T) {
	t.Parallel()

	registered := make([]string, 0, len(wantRoutes))

	for _, route := range newBrowser(t).app.GetRoutes(true) {
		registered = append(registered, route.Method+" "+route.Path)
	}

	want := make([]string, 0, len(wantRoutes))

	for _, route := range wantRoutes {
		want = append(want, route.method+" "+route.path)
	}

	assert.Len(t, registered, len(wantRoutes), "one registered route per expected row")
	assert.ElementsMatch(t, want, registered)
}

func TestNewKeepsTheClipRouteConstantAndItsLiteralInSync(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/clips", routeClips)

	registered := make([]string, 0, len(wantRoutes))

	for _, route := range newBrowser(t).app.GetRoutes(true) {
		registered = append(registered, route.Method+" "+route.Path)
	}

	assert.Contains(t, registered, http.MethodGet+" "+"/clips")
	assert.Contains(t, registered, http.MethodGet+" "+"/api/clips")
	assert.Contains(t, registered, http.MethodPost+" "+"/api/clips")
}

func TestNewMountsTheAuthGuardOnEveryGuardedRoute(t *testing.T) {
	t.Parallel()

	handlers := make(map[string]int)

	for _, route := range newBrowser(t).app.GetRoutes(true) {
		handlers[route.Method+" "+route.Path] = len(route.Handlers)
	}

	for _, route := range wantRoutes {
		want := 1
		if route.guarded {
			want = 2
		}

		assert.Equal(t, want, handlers[route.method+" "+route.path],
			"%s %s", route.method, route.path)
	}
}

func TestNewBlocksAnonymousRequestsToEveryGuardedRoute(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, route := range wantRoutes {
		if !route.guarded {
			continue
		}

		got := client.send(t, route)

		if strings.HasPrefix(route.path, "/api/") {
			assert.Equal(t, fiber.StatusUnauthorized, got.status,
				"%s %s", route.method, route.path)

			continue
		}

		assert.Equal(t, fiber.StatusSeeOther, got.status,
			"%s %s", route.method, route.path)
		assert.Equal(t, "/login", got.location, route.path)
	}
}

func TestNewAdmitsAuthenticatedRequestsToEveryGuardedRoute(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.authenticate(t)

	for _, route := range wantRoutes {
		if !route.guarded {
			continue
		}

		got := client.send(t, route)

		assert.NotEqual(t, fiber.StatusUnauthorized, got.status,
			"%s %s", route.method, route.path)
		assert.NotEqual(t, "/login", got.location,
			"%s %s", route.method, route.path)
	}
}

func TestNewKeepsASecondBrowserAnonymous(t *testing.T) {
	t.Parallel()

	owner := newBrowser(t)
	owner.authenticate(t)

	stranger := owner.sibling(t)

	got := stranger.request(t, http.MethodGet, "/")

	assert.Equal(t, fiber.StatusSeeOther, got.status,
		"signing in one browser does not sign in every other")
	assert.Equal(t, "/login", got.location)
}

func TestNewRefusesASecondPlexAccount(t *testing.T) {
	t.Parallel()

	owner := newBrowser(t)
	owner.authenticate(t)

	intruder := owner.sibling(t)

	got := intruder.submit(t, "/api/auth/login", url.Values{"token": {otherToken}})

	require.Equal(t, fiber.StatusSeeOther, got.status)
	assert.True(t, strings.HasPrefix(got.location, "/login?"),
		"the second account is sent back to the login page, not %q", got.location)
	assert.Equal(t, "/login", intruder.request(t, http.MethodGet, "/").location,
		"the refused account holds no session")
}

func TestNewLogoutEndsOnlyThatBrowsersSession(t *testing.T) {
	t.Parallel()

	first := newBrowser(t)
	first.authenticate(t)

	second := first.sibling(t)
	second.authenticate(t)

	got := first.submit(t, "/api/auth/logout", nil)

	require.Equal(t, fiber.StatusSeeOther, got.status)
	assert.Equal(t, "/login", got.location)
	assert.Equal(t, "/login", first.request(t, http.MethodGet, "/").location,
		"the browser that logged out is anonymous again")
	assert.Equal(t, fiber.StatusOK, second.request(t, http.MethodGet, "/").status,
		"the other browser stays signed in")

	owned, err := first.db.HasOwner(t.Context())
	require.NoError(t, err)
	assert.True(t, owned, "logging out keeps the owner")
}

func TestNewLogsOutAnExpiredSessionToo(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	got := client.submit(t, "/api/auth/logout", nil)

	assert.Equal(t, fiber.StatusSeeOther, got.status,
		"logging out never needs a live session, only a CSRF token")
	assert.Equal(t, "/login", got.location)
}

func TestNewRefusesAForeignHost(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)

	client.host = "rebind.attacker.example"

	for _, target := range []string{"/login", "/api/auth/status", "/assets/css/output.css"} {
		got := client.request(t, http.MethodGet, target)

		assert.Equal(t, fiber.StatusMisdirectedRequest, got.status, target)
	}

	assert.Equal(t, fiber.StatusOK, client.request(t, http.MethodGet, "/api/healthz").status,
		"probes reach the health check under any host")
}

func TestNewServesTheOpenRoutesWithoutASession(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, route := range wantRoutes {
		if route.guarded {
			continue
		}

		got := client.send(t, route)

		assert.NotEqual(t, fiber.StatusUnauthorized, got.status,
			"%s %s", route.method, route.path)
	}
}

func TestNewAnswersTheHealthCheckWithoutASession(t *testing.T) {
	t.Parallel()

	got := newBrowser(t).request(t, http.MethodGet, "/api/healthz")

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.JSONEq(t, `{"status":"ok","service":"outtake"}`, got.body)
}

func TestNewRendersTheSessionPagesOnceSignedIn(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.authenticate(t)

	for _, target := range []string{"/", routeClips, "/media", "/settings/profiles"} {
		got := client.request(t, http.MethodGet, target)

		assert.Equal(t, fiber.StatusOK, got.status, target)
	}
}

func TestNewResolvesTheMediaItemRoutesWithoutShadowing(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, target := range []string{"/media/item/5", "/media/item/5/clips", "/media/item/5/playback"} {
		got := client.request(t, http.MethodGet, target)

		assert.Equal(t, fiber.StatusSeeOther, got.status, target)
		assert.Equal(t, "/login", got.location, target)
	}

	got := client.request(t, http.MethodGet, "/media/item/5/unknown")

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"no media item route claims an extra path segment")
}

func TestNewResolvesTheClipRoutesWithoutShadowing(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, target := range []string{routeClips, "/clips/new", "/clips/7/file", "/clips/7/row"} {
		got := client.request(t, http.MethodGet, target)

		assert.Equal(t, fiber.StatusSeeOther, got.status, target)
		assert.Equal(t, "/login", got.location, target)
	}

	got := client.request(t, http.MethodGet, "/clips/7/unknown")

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"the clip parameter is not a catch-all")
}

func TestNewResolvesTheAPIClipRoutesWithoutShadowing(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, route := range []routerRoute{
		{method: http.MethodGet, path: "/api/clips", sample: "/api/clips"},
		{method: http.MethodPost, path: "/api/clips/preview", sample: "/api/clips/preview"},
		{method: http.MethodGet, path: "/api/clips/preview/:id", sample: "/api/clips/preview/abc"},
		{method: http.MethodDelete, path: "/api/clips/preview/:id", sample: "/api/clips/preview/abc"},
		{method: http.MethodPost, path: "/api/clips/:id/update", sample: "/api/clips/abc/update"},
		{method: http.MethodPost, path: "/api/clips/:id/cancel", sample: "/api/clips/abc/cancel"},
		{method: http.MethodGet, path: "/api/clips/:id/status", sample: "/api/clips/abc/status"},
		{method: http.MethodGet, path: "/api/clips/:id/download", sample: "/api/clips/abc/download"},
		{method: http.MethodDelete, path: "/api/clips/:id", sample: "/api/clips/abc"},
	} {
		got := client.send(t, route)

		assert.Equal(t, fiber.StatusUnauthorized, got.status,
			"%s %s", route.method, route.path)
	}

	got := client.request(t, http.MethodDelete, "/api/clips/abc/unknown")

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"the API clip parameter is not a catch-all")
}

func TestNewRejectsAnUnregisteredPath(t *testing.T) {
	t.Parallel()

	got := newBrowser(t).request(t, http.MethodGet, "/no/such/page")

	assert.Equal(t, fiber.StatusNotFound, got.status)
}

func TestNewRejectsAWrongMethodOnARegisteredPath(t *testing.T) {
	t.Parallel()

	got := newBrowser(t).request(t, http.MethodGet, "/api/auth/logout")

	assert.Equal(t, fiber.StatusMethodNotAllowed, got.status)
}

func TestNewServesTheEmbeddedAssetsWithoutASession(t *testing.T) {
	t.Parallel()

	got := newBrowser(t).request(t, http.MethodGet, "/assets/css/output.css")

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.Contains(t, got.contentType, "text/css")
	assert.NotEmpty(t, got.body)
}

func TestNewRouterHandlersBuildsEveryMountedHandler(t *testing.T) {
	t.Parallel()

	built := newRouterHandlers(testRouterDeps(t, testRouterDatabase(t)))

	assert.NotNil(t, built.clip)
	assert.NotNil(t, built.preview)
	assert.NotNil(t, built.media)
	assert.NotNil(t, built.auth)
	assert.NotNil(t, built.home)
	assert.NotNil(t, built.library)
	assert.NotNil(t, built.server)
	assert.NotNil(t, built.thumb)
	assert.NotNil(t, built.profiles)
	assert.NotNil(t, built.health)
}

// authenticate signs the browser in as the owner through the login form,
// against the plex.tv stand-in.
//
// Parameters:
//   - t: The test that signs in.
func (b *browser) authenticate(t *testing.T) {
	t.Helper()

	b.start(t)

	got := b.submit(t, "/api/auth/login", url.Values{"token": {ownerToken}})
	require.Equal(t, fiber.StatusSeeOther, got.status)
	require.False(t, strings.HasPrefix(got.location, "/login"),
		"the sign-in was refused: %s", got.location)
}

// do sends a form request the way a browser would, keeping the cookies the
// response sets.
//
// Parameters:
//   - t: The test that issues the request.
//   - method: HTTP method of the request.
//   - target: Request path.
//   - form: Form body to send.
//
// Returns:
//   - got: The response the router produced.
func (b *browser) do(t *testing.T, method, target string, form url.Values) answer {
	t.Helper()

	req := httptest.NewRequestWithContext(
		t.Context(),
		method,
		target,
		strings.NewReader(form.Encode()),
	)

	req.Host = b.host
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	req.Header.Set("X-Csrf-Token", b.csrfToken)

	for _, cookie := range b.cookies {
		req.AddCookie(cookie)
	}

	resp, err := b.app.Test(req)
	require.NoError(t, err)

	got := readAnswer(t, resp)
	b.keep(got.cookies)

	return got
}

// keep stores the cookies a response set, replacing any of the same name the
// way a browser does, and follows a CSRF token the response rotated.
//
// Parameters:
//   - set: Cookies the response set.
func (b *browser) keep(set []*http.Cookie) {
	for _, cookie := range set {
		b.cookies = slices.DeleteFunc(b.cookies, func(held *http.Cookie) bool {
			return held.Name == cookie.Name
		})
		b.cookies = append(b.cookies, cookie)

		if cookie.Name == "csrf_" {
			b.csrfToken = cookie.Value
		}
	}
}

// request issues one request, carrying the session cookies and the CSRF token.
//
// Parameters:
//   - t: The test that issues the request.
//   - method: HTTP method of the request.
//   - target: Request path.
//
// Returns:
//   - got: The response the router produced.
func (b *browser) request(t *testing.T, method, target string) answer {
	t.Helper()

	form := url.Values{}
	form.Set("_csrf", b.csrfToken)

	return b.do(t, method, target, form)
}

// send issues the request a route row stands for.
//
// Parameters:
//   - t: The test that issues the request.
//   - route: The route row naming the method and the concrete path.
//
// Returns:
//   - got: The response the router produced.
func (b *browser) send(t *testing.T, route routerRoute) answer {
	t.Helper()

	return b.request(t, route.method, route.sample)
}

// sibling builds a second browser against the same router and database, with
// cookies of its own.
//
// Parameters:
//   - t: The test that owns the browser.
//
// Returns:
//   - other: A browser that shares nothing with b but the server.
func (b *browser) sibling(t *testing.T) *browser {
	t.Helper()

	other := &browser{app: b.app, db: b.db, cookies: nil, csrfToken: "", host: routerHost}
	other.start(t)

	return other
}

// start primes the cookies and the CSRF token from the login page.
//
// Parameters:
//   - t: The test that primes the browser.
func (b *browser) start(t *testing.T) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", http.NoBody)

	req.Host = b.host

	resp, err := b.app.Test(req)
	require.NoError(t, err)

	b.keep(readAnswer(t, resp).cookies)

	require.NotEmpty(t, b.csrfToken, "the login page issued a CSRF token")
}

// submit posts a form carrying the CSRF token.
//
// Parameters:
//   - t: The test that posts the form.
//   - target: Request path.
//   - fields: Form fields beyond the CSRF token, which may be nil.
//
// Returns:
//   - got: The response the router produced.
func (b *browser) submit(t *testing.T, target string, fields url.Values) answer {
	t.Helper()

	form := url.Values{}
	maps.Copy(form, fields)
	form.Set("_csrf", b.csrfToken)

	return b.do(t, http.MethodPost, target, form)
}

// readAnswer drains a response and closes it.
//
// Parameters:
//   - t: The test that read the response.
//   - resp: The response to observe.
//
// Returns:
//   - got: Everything the test observes about the response.
func readAnswer(t *testing.T, resp *http.Response) answer {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return answer{
		status:      resp.StatusCode,
		location:    resp.Header.Get(fiber.HeaderLocation),
		contentType: resp.Header.Get(fiber.HeaderContentType),
		body:        string(body),
		cookies:     resp.Cookies(),
	}
}

// newBrowser builds the router over a throwaway database.
//
// Parameters:
//   - t: The test that owns the router.
//
// Returns:
//   - client: A browser wired to a freshly built router.
func newBrowser(t *testing.T) *browser {
	t.Helper()

	db := testRouterDatabase(t)

	return &browser{
		app:       New(testRouterDeps(t, db)),
		db:        db,
		cookies:   nil,
		csrfToken: "",
		host:      routerHost,
	}
}

// testRouterDatabase opens a migrated SQLite database inside a temp directory.
//
// Parameters:
//   - t: The test that owns the database.
//
// Returns:
//   - db: A database handle closed when the test finishes.
func testRouterDatabase(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.New(filepath.Join(t.TempDir(), "outtake.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// testRouterDeps builds the smallest dependency set New accepts. The queue is
// created but never started, so it holds no jobs and runs no workers.
//
// Parameters:
//   - t: The test that owns the dependencies.
//   - db: Database the middleware and handlers read through.
//
// Returns:
//   - deps: Dependencies wired to real but idle collaborators.
func testRouterDeps(t *testing.T, db *database.DB) Deps {
	t.Helper()

	cfg := &config.Config{
		ListenAddr:  "127.0.0.1:8080",
		LogLevel:    "info",
		Env:         "test",
		SessionPoll: 10 * time.Second,
	}

	return Deps{
		Cfg:   cfg,
		DB:    db,
		Queue: queue.NewQueue(1, nil),
		Auth: identity.New(
			"outtake", "test-client", "http://localhost", db, nil,
			identity.WithPlexURL(routerPlexTV(t).URL),
		),
	}
}

// routerPlexTV stands in for plex.tv. ownerToken belongs to account 42 and
// otherToken to account 7. Every other request is refused.
//
// Parameters:
//   - t: The test that owns the stand-in.
//
// Returns:
//   - server: The running plex.tv stand-in.
func routerPlexTV(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/user", func(writer http.ResponseWriter, request *http.Request) {
		switch request.Header.Get("X-Plex-Token") {
		case ownerToken:
			_, _ = writer.Write([]byte(`{"id": 42, "username": "owner"}`))
		case otherToken:
			_, _ = writer.Write([]byte(`{"id": 7, "username": "other"}`))
		default:
			writer.WriteHeader(http.StatusUnauthorized)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}
