// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	identitymocks "github.com/PapagoLabs/outtake/internal/plex/identity/mocks"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

type authAnswer struct {
	status int
	header http.Header
	body   string
}
type authBrowser struct {
	t       *testing.T
	app     *fiber.App
	cookies []*http.Cookie
}

// authAnswer is what one served auth request produced.

// sessionApp builds an app carrying nothing but the session middleware.
//
// Parameters:
//   - t: The test the app belongs to.
//
// Returns:
//   - app: The app routes are mounted on.
func sessionApp(t *testing.T) *fiber.App {
	t.Helper()

	middleware, store := session.NewWithStore(session.Config{})
	require.NotNil(t, store)

	app := fiber.New()
	app.Use(middleware)

	return app
}

// authApp builds an app whose Plex calls cannot leave the process. The identity
// package builds every Plex client against plex.tv and exposes no seam for
// pointing it elsewhere, so a canceled request context is what keeps those calls
// from being sent at all.
func authApp(t *testing.T) *fiber.App {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	app := sessionApp(t)
	app.Use(func(c fiber.Ctx) error {
		c.SetContext(ctx)

		return c.Next()
	})

	return app
}

// authHandler builds a Plex authentication service around the collaborators.
//
// Parameters:
//   - t: The test the service belongs to.
//   - store: Token store the service persists through, which may be nil.
//   - selected: Plex server binding, which may be nil.
//
// Returns:
//   - handler: The auth handler under test.
func authHandler(
	t *testing.T,
	store identity.TokenStore,
	selected identity.ServerBinding,
) *Handler {
	t.Helper()

	return New(identity.New("outtake", "test-client", "http://localhost", store, selected))
}

// browser issues requests against an app, carrying the session cookie between
// them so a value stored by one request is visible to the next.
//
// Parameters:
//   - t: The test the browser belongs to.
//   - app: The app to serve.
//
// Returns:
//   - browser: The cookie-carrying request issuer.
func newBrowser(t *testing.T, app *fiber.App) *authBrowser {
	t.Helper()

	return &authBrowser{t: t, app: app}
}

// authBrowser issues requests that share one session.

// do issues one request against the app.
//
// Parameters:
//   - method: HTTP method to issue.
//   - target: Request target, including any query string.
//   - form: Form body, empty for none.
//   - htmx: Whether to mark the request as coming from HTMX.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func (browser *authBrowser) do(
	method, target, form string,
	htmx bool,
) authAnswer {
	browser.t.Helper()

	var req *http.Request

	if form == "" {
		req = httptest.NewRequestWithContext(browser.t.Context(), method, target, nil)
	} else {
		req = httptest.NewRequestWithContext(
			browser.t.Context(), method, target, strings.NewReader(form),
		)
		req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")
	}

	if htmx {
		req.Header.Set(routes.HeaderHXRequest, "true")
	}

	for _, cookie := range browser.cookies {
		req.AddCookie(cookie)
	}

	resp, err := browser.app.Test(req)
	require.NoError(browser.t, err)

	defer closeBody(browser.t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(browser.t, err)

	browser.cookies = append(browser.cookies, resp.Cookies()...)

	return authAnswer{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

// seedSession stores values on the session later requests will see.
//
// Parameters:
//   - t: The test the session belongs to.
//   - browser: The browser that will issue the later requests.
//   - values: Session values to store, empty to store nothing.
func seedSession(t *testing.T, browser *authBrowser, values map[string]any) {
	t.Helper()

	var stored bool

	browser.app.Get("/session/seed", func(ctx fiber.Ctx) error {
		sess := session.FromContext(ctx)
		for key, value := range values {
			sess.Set(key, value)
		}

		stored = true

		return ctx.SendStatus(fiber.StatusOK)
	})

	browser.do(http.MethodGet, "/session/seed", "", false)

	require.True(t, stored, "the seed route has to run for the values to be stored")
}

// readSession reports what one request left on the session.
//
// Parameters:
//   - t: The test the session belongs to.
//   - browser: The browser carrying the session cookie.
//   - read: Reader invoked with the live session.
//
// Returns:
//   - got: The value the reader produced.
func readSession(
	t *testing.T,
	browser *authBrowser,
	read func(sess *session.Middleware) any,
) any {
	t.Helper()

	var got any

	browser.app.Get("/session/read", func(ctx fiber.Ctx) error {
		got = read(session.FromContext(ctx))

		return ctx.SendStatus(fiber.StatusOK)
	})

	browser.do(http.MethodGet, "/session/read", "", false)

	return got
}

// pendingPIN is a session carrying a Plex PIN awaiting authorization.
func pendingPIN() map[string]any {
	return map[string]any{
		identity.SessionKeyPinID:   4321,
		identity.SessionKeyPinCode: "PIN-CODE",
	}
}

// flashOf reads the path and flash text a redirect carries.
//
// Parameters:
//   - t: The test the redirect belongs to.
//   - location: The redirect target.
//
// Returns:
//   - path: The path the redirect lands on.
//   - flash: The flash text, empty when none was carried.
func flashOf(t *testing.T, location string) (string, string) {
	t.Helper()

	parsed, err := url.Parse(location)
	require.NoError(t, err)

	return parsed.Path, parsed.Query().Get(routes.QueryError)
}

func TestNewKeepsTheServiceItWasGiven(t *testing.T) {
	t.Parallel()

	service := identity.New("outtake", "test-client", "http://localhost", nil, nil)

	assert.Equal(t, &Handler{auth: service}, New(service))
}

func TestLoginSendsAnAlreadyAuthenticatedBrowserToTheDashboard(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Post("/api/auth/login", authHandler(t, nil, nil).Login)

	browser := newBrowser(t, app)

	seedSession(t, browser, map[string]any{identity.SessionKeyToken: "already-in"})

	answer := browser.do(http.MethodPost, "/api/auth/login", "", false)

	path, _ := flashOf(t, answer.header.Get(fiber.HeaderLocation))
	assert.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, path,
		"a browser that already holds a token has nothing to log in to")
}

func TestLoginRejectsATokenPlexWillNotAccept(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Post("/api/auth/login", authHandler(t, nil, nil).Login)

	form := url.Values{"token": {"typed-token"}}.Encode()

	browser := newBrowser(t, app)

	answer := browser.do(http.MethodPost, "/api/auth/login", form, false)

	path, flash := flashOf(t, answer.header.Get(fiber.HeaderLocation))
	assert.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathLogin, path)
	assert.Equal(t, msgInvalidPlexToken, flash,
		"the user is told Plex refused the token, not that the request failed")
}

func TestLoginKeepsARejectedTokenOutOfTheSessionAndTheStore(t *testing.T) {
	t.Parallel()

	store := identitymocks.NewMockTokenStore(t)

	app := authApp(t)
	app.Post("/api/auth/login", authHandler(t, store, nil).Login)

	form := url.Values{"token": {"typed-token"}}.Encode()
	browser := newBrowser(t, app)

	browser.do(http.MethodPost, "/api/auth/login", form, false)

	got := readSession(t, browser, func(sess *session.Middleware) any {
		userID, _ := sess.Get(identity.SessionKeyUserID).(int)

		return []any{identity.Token(sess), userID}
	})

	assert.Equal(t, []any{"", 0}, got,
		"a token Plex refused never reaches the session")
}

func TestLoginReportsAPINItCouldNotCreate(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Post("/api/auth/login", authHandler(t, nil, nil).Login)

	browser := newBrowser(t, app)

	answer := browser.do(http.MethodPost, "/api/auth/login", "", false)

	assert.Equal(t, fiber.StatusBadGateway, answer.status)
	assert.Contains(t, answer.body, api.PINFailed)
	assert.NotContains(t, answer.body, "authUrl",
		"a PIN that was never created has no authorization URL to send the browser to")
}

func TestCallbackRejectsASessionWithNoPENDINGPIN(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Get("/api/auth/callback", authHandler(t, nil, nil).Callback)

	browser := newBrowser(t, app)

	answer := browser.do(http.MethodGet, "/api/auth/callback", "", false)

	path, flash := flashOf(t, answer.header.Get(fiber.HeaderLocation))
	assert.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathLogin, path)
	assert.Equal(t, "No PIN session", flash)
}

func TestCallbackReportsAPINThatIsNotYetAuthorized(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Get("/api/auth/callback", authHandler(t, nil, nil).Callback)

	browser := newBrowser(t, app)

	seedSession(t, browser, pendingPIN())

	answer := browser.do(http.MethodGet, "/api/auth/callback", "", false)

	path, flash := flashOf(t, answer.header.Get(fiber.HeaderLocation))
	assert.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathLogin, path)
	assert.Equal(t, "PIN not yet authorized", flash,
		"the browser is told to wait rather than that the login is broken")
}

func TestCallbackKeepsThePendingPINWhenItFails(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Get("/api/auth/callback", authHandler(t, nil, nil).Callback)

	browser := newBrowser(t, app)

	seedSession(t, browser, pendingPIN())
	browser.do(http.MethodGet, "/api/auth/callback", "", false)

	got := readSession(t, browser, func(sess *session.Middleware) any {
		pinID, pinCode := identity.StoredPIN(sess)

		return []any{pinID, pinCode}
	})

	assert.Equal(t, []any{4321, "PIN-CODE"}, got,
		"a poll that came back unclaimed has to be able to ask again")
}

func TestStatusTellsAnAuthenticatedBrowserToGoOn(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Get("/api/auth/status", authHandler(t, nil, nil).Status)

	browser := newBrowser(t, app)

	seedSession(t, browser, map[string]any{identity.SessionKeyToken: "already-in"})

	answer := browser.do(http.MethodGet, "/api/auth/status", "", true)

	assert.Equal(t, fiber.StatusOK, answer.status)
	assert.Equal(t, statusAuthed, answer.body)
	assert.Equal(t, routes.PathRoot, answer.header.Get("Hx-Redirect"))
}

func TestStatusWaitsWhenNoPINISPending(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Get("/api/auth/status", authHandler(t, nil, nil).Status)

	browser := newBrowser(t, app)

	answer := browser.do(http.MethodGet, "/api/auth/status", "", true)

	assert.Equal(t, fiber.StatusOK, answer.status)
	assert.Equal(t, statusWaiting, answer.body)
	assert.Empty(t, answer.header.Get("Hx-Redirect"),
		"there is nothing to go to yet, so the page keeps polling")
}

func TestStatusWaitsWhilePlexHasNotAuthorizedThePIN(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Get("/api/auth/status", authHandler(t, nil, nil).Status)

	browser := newBrowser(t, app)

	seedSession(t, browser, pendingPIN())

	answer := browser.do(http.MethodGet, "/api/auth/status", "", true)

	assert.Equal(t, fiber.StatusOK, answer.status)
	assert.Equal(t, statusWaiting, answer.body,
		"a poll that comes back unclaimed is still waiting, not finished")
	assert.Empty(t, answer.header.Get("Hx-Redirect"))
}

func TestStatusKeepsThePendingPINWhilePlexHasNotAnswered(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Get("/api/auth/status", authHandler(t, nil, nil).Status)

	browser := newBrowser(t, app)

	seedSession(t, browser, pendingPIN())
	browser.do(http.MethodGet, "/api/auth/status", "", true)

	got := readSession(t, browser, func(sess *session.Middleware) any {
		pinID, pinCode := identity.StoredPIN(sess)

		return []any{pinID, pinCode}
	})

	assert.Equal(t, []any{4321, "PIN-CODE"}, got,
		"the next poll has to be able to ask about the same PIN")
}

func TestStatusReportsAFailedLogout(t *testing.T) {
	t.Parallel()

	store := identitymocks.NewMockTokenStore(t)
	store.EXPECT().ClearAuth(mock.Anything).Return(errStoreClosed)

	app := authApp(t)
	app.Post("/api/auth/logout", authHandler(t, store, nil).Logout)

	browser := newBrowser(t, app)

	answer := browser.do(http.MethodPost, "/api/auth/logout", "", false)

	assert.Equal(t, fiber.StatusInternalServerError, answer.status)
	assert.Contains(t, answer.body, api.LogoutFailed)
}

func TestPostAuthPathSendsAnUnboundInstallToTheServerPicker(t *testing.T) {
	t.Parallel()

	binding := identitymocks.NewMockServerBinding(t)
	binding.EXPECT().Get().Return(plex.EmptyServer(), false)

	assert.Equal(t, routes.PathServers, authHandler(t, nil, binding).postAuthPath(),
		"a login with no server behind it still has to pick one")
}

func TestPostAuthPathSendsABoundInstallToTheDashboard(t *testing.T) {
	t.Parallel()

	binding := identitymocks.NewMockServerBinding(t)
	binding.EXPECT().Get().Return(plex.Server{Name: "loopback"}, true)

	assert.Equal(t, routes.PathRoot, authHandler(t, nil, binding).postAuthPath(),
		"a server is already bound, so there is nothing to pick")
}

func TestPostAuthPathToleratesNoBindingAtAll(t *testing.T) {
	t.Parallel()

	assert.Equal(t, routes.PathServers, authHandler(t, nil, nil).postAuthPath())
}

func TestFinishAuthAuthenticatesEvenWhenTheTokenCannotBePersisted(t *testing.T) {
	t.Parallel()

	store := identitymocks.NewMockTokenStore(t)
	store.EXPECT().SaveToken(mock.Anything, "test-client", "typed-token").
		Return(errStoreClosed)

	binding := identitymocks.NewMockServerBinding(t)
	binding.EXPECT().Get().Return(plex.EmptyServer(), false)

	app := authApp(t)
	app.Post("/finish", func(ctx fiber.Ctx) error {
		authHandler(t, store, binding).
			finishAuth(ctx, session.FromContext(ctx), "typed-token")

		return ctx.SendStatus(fiber.StatusOK)
	})

	browser := newBrowser(t, app)

	browser.do(http.MethodPost, "/finish", "", false)

	got := readSession(t, browser, func(sess *session.Middleware) any {
		userID, _ := sess.Get(identity.SessionKeyUserID).(int)

		return []any{identity.Token(sess), userID}
	})

	assert.Equal(t, []any{"typed-token", 0}, got,
		"a token that could not be written still authenticates the session")
}

func TestFinishAuthStoresTheTokenOnTheSession(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Post("/finish", func(ctx fiber.Ctx) error {
		authHandler(t, nil, nil).
			finishAuth(ctx, session.FromContext(ctx), "typed-token")

		return ctx.SendStatus(fiber.StatusOK)
	})

	browser := newBrowser(t, app)

	browser.do(http.MethodPost, "/finish", "", false)

	assert.Equal(t, "typed-token",
		readSession(t, browser, func(sess *session.Middleware) any {
			return identity.Token(sess)
		}),
		"the session carries the token Plex handed out")
}

func TestSendAuthCompleteReportsARenderFailure(t *testing.T) {
	t.Parallel()

	app := authApp(t)
	app.Get("/done", func(ctx fiber.Ctx) error {
		return sendAuthComplete(ctx, routes.PathRoot)
	})

	browser := newBrowser(t, app)

	answer := browser.do(http.MethodGet, "/done", "", false)

	assert.NotEqual(t, fiber.StatusOK, answer.status,
		"a render that could not finish must not look like a finished login")
	assert.Contains(t, answer.body, "send auth complete",
		"the failure names the step that could not be done")
}

func TestWrapAuthPassesNilThrough(t *testing.T) {
	t.Parallel()

	assert.NoError(t, wrapAuth(nil, "finish auth"))
}

func TestWrapAuthNamesTheOperationItFailedIn(t *testing.T) {
	t.Parallel()

	wrapped := wrapAuth(assert.AnError, "finish auth")

	require.Error(t, wrapped)
	assert.Contains(t, wrapped.Error(), "finish auth")
	assert.ErrorIs(t, wrapped, assert.AnError,
		"the cause has to survive so a caller can still match on it")
}

func TestSendAuthCompleteRendersTheHandoff(t *testing.T) {
	t.Parallel()

	app := sessionApp(t)
	app.Get("/done", func(ctx fiber.Ctx) error {
		return sendAuthComplete(ctx, routes.PathRoot)
	})

	browser := newBrowser(t, app)

	answer := browser.do(http.MethodGet, "/done", "", false)

	assert.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, routes.PathRoot,
		"the page the browser lands on has to be in the answer")
}
