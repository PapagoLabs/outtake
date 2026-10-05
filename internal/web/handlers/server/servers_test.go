// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/server/mocks"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// serversRoutes are the routes the server picker serves, as a table a session
// carrying app can install.
var serversRoutes = []struct {
	method  string
	path    string
	handler func(*Handler) fiber.Handler
}{
	{
		method: http.MethodGet, path: routes.PathServers,
		handler: func(h *Handler) fiber.Handler { return h.Servers },
	},
	{
		method: http.MethodPost, path: routes.PathServers + "/select",
		handler: func(h *Handler) fiber.Handler { return h.SelectServer },
	},
	{
		method: http.MethodGet, path: "/bind",
		handler: func(h *Handler) fiber.Handler { return h.bindSelectedURL },
	},
	{
		method: http.MethodGet, path: "/discover",
		handler: func(h *Handler) fiber.Handler {
			return func(ctx fiber.Ctx) error {
				return ctx.JSON(h.discoverServers(ctx))
			}
		},
	},
}

// errListFailed reports a Plex account whose servers cannot be listed.
var errListFailed = errors.New("servers unavailable")

// errSelectFailed reports a Plex account that will not take the server.
var errSelectFailed = errors.New("select server")

// serversApp mounts the server picker routes on a fresh app.
//
// Parameters:
//   - handler: The handler under test.
//
// Returns:
//   - app: The app the routes are mounted on.
func serversApp(handler *Handler) *fiber.App {
	app := fiber.New()
	app.Get(routes.PathServers, handler.Servers)
	app.Post(routes.PathServers+"/select", handler.SelectServer)

	return app
}

// sessionAppWithRoutes builds an app carrying the session middleware with the
// given routes installed on it.
//
// Parameters:
//   - t: The test the app belongs to.
//   - handler: The handler under test.
//   - routes: Routes to install.
//
// Returns:
//   - app: The app the routes are mounted on.
func sessionAppWithRoutes(
	t *testing.T,
	handler *Handler,
	routes []struct {
		method  string
		path    string
		handler func(*Handler) fiber.Handler
	},
) *fiber.App {
	t.Helper()

	app := fiber.New()
	app.Use(sessionMiddleware(t))

	for _, route := range routes {
		app.Add([]string{route.method}, route.path, route.handler(handler))
	}

	return app
}

// getServers serves one server picker request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func getServers(t *testing.T, handler *Handler) pageAnswer {
	t.Helper()

	return serve(t, serversApp(handler), routes.PathServers, false)
}

// selectServer serves one server selection request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - form: Form body to post, empty for none.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func selectServer(t *testing.T, handler *Handler, form string) pageAnswer {
	t.Helper()

	return serveMethod(t, serversApp(handler), http.MethodPost, routes.PathServers+"/select",
		false, "", form)
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

// discoveredAuth builds an authentication whose Plex account lists one server.
//
// Parameters:
//   - t: The test the authentication belongs to.
//   - err: Failure the account listing reports, empty for none.
//
// Returns:
//   - auth: The authentication under test.
func discoveredAuth(t *testing.T, err error) *mocks.MockPlexAuth {
	t.Helper()

	stub := startPMS(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(
			[]byte(
				`[{"name":"Home Plex","clientIdentifier":"machine-1","provides":"server","owned":true,"accessToken":"page-token"}]`,
			),
		)
	})

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Client().Return(stub.client, stub.server, true).Maybe()
	auth.EXPECT().Selected().Return(plex.EmptyServer(), false).Maybe()

	if err != nil {
		auth.EXPECT().
			Discover(mock.Anything, mock.Anything).
			Return(nil, err).
			Maybe()

		return auth
	}

	auth.EXPECT().
		Discover(mock.Anything, mock.Anything).
		Return([]plex.Server{{
			Name:    "Home Plex",
			Address: "10.0.0.5",
			Port:    32400,
			Scheme:  "http",
			Token:   "page-token",
		}}, nil).
		Maybe()

	return auth
}

// selectedAuth builds an authentication that accepts any server selection.
//
// Parameters:
//   - t: The test the authentication belongs to.
//   - err: Failure the selection reports, nil to accept.
//
// Returns:
//   - auth: The authentication under test.
func selectedAuth(t *testing.T, err error) *mocks.MockPlexAuth {
	t.Helper()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Select(mock.Anything, mock.Anything).Return(err).Maybe()
	auth.EXPECT().Selected().Return(plex.EmptyServer(), false).Maybe()

	return auth
}

func TestServersRendersTheDiscoveredAccount(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, discoveredAuth(t, nil), silentSources(t))

	app := fiber.New()
	app.Use(sessionMiddleware(t))
	app.Get(routes.PathServers, handler.Servers)

	cookies := seedTokenFor(t, app, "session-token")

	answer := serveWithCookies(t, app, cookies, routes.PathServers, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Home Plex",
		"a server the account offers has to be on the page")
	assertBodyContains(t, answer.body, `name="token"`,
		"the form carries the token Plex needs to reach the account")
}

func TestServersWarnsWhenTheAccountCannotBeListed(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, discoveredAuth(t, errListFailed), silentSources(t))

	app := fiber.New()
	app.Use(sessionMiddleware(t))
	app.Get(routes.PathServers, handler.Servers)

	cookies := seedTokenFor(t, app, "session-token")

	answer := serveWithCookies(t, app, cookies, routes.PathServers, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyOmits(t, answer.body, "Home Plex",
		"no server was listed, so none is offered")
	assertBodyContains(t, answer.body, `name="customUrl"`,
		"a user can still type a server address by hand")
}

func TestServersWarnsWithNoServerBound(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := getServers(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `name="customUrl"`,
		"with no Plex bound, typing an address is the only way forward")
}

func TestSelectServerBindsACustomURL(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Select(mock.Anything, mock.MatchedBy(func(server plex.Server) bool {
		return server.Address == "10.0.0.9" && server.Port == 32400 &&
			server.Scheme == "http" && server.Token == "form-token"
	})).Return(nil)

	handler := pageHandler(t, auth, silentSources(t))

	form := url.Values{
		"customUrl": {"http://10.0.0.9:32400"},
		"token":     {"form-token"},
	}.Encode()

	answer := selectServer(t, handler, form)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, answer.header.Get(fiber.HeaderLocation),
		"a bound install goes straight to the dashboard")
}

func TestSelectServerAppliesTheNameTheUserTyped(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Select(mock.Anything, mock.MatchedBy(func(server plex.Server) bool {
		return server.Name == "Home Plex"
	})).Return(nil)

	handler := pageHandler(t, auth, silentSources(t))

	form := url.Values{
		"customUrl": {"http://10.0.0.9:32400"},
		"name":      {"Home Plex"},
		"token":     {"form-token"},
	}.Encode()

	answer := selectServer(t, handler, form)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
}

func TestSelectServerBindsAServerPartsURL(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Select(mock.Anything, mock.MatchedBy(func(server plex.Server) bool {
		return server.Scheme == "https" && server.Address == "plex.example.com" &&
			server.Port == 32400 && server.Token == "form-token"
	})).Return(nil)

	handler := pageHandler(t, auth, silentSources(t))

	form := url.Values{
		"scheme":  {"https"},
		"address": {"plex.example.com"},
		"port":    {"32400"},
		"token":   {"form-token"},
	}.Encode()

	answer := selectServer(t, handler, form)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, answer.header.Get(fiber.HeaderLocation))
}

func TestSelectServerRejectsAnAddressItCannotRead(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, offlineAuth(t), silentSources(t))

	form := url.Values{
		"customUrl": {"not a url"},
		"token":     {"form-token"},
	}.Encode()

	answer := selectServer(t, handler, form)

	require.Equal(t, fiber.StatusSeeOther, answer.status)

	path, flash := flashOf(t, answer.header.Get(fiber.HeaderLocation))
	assert.Equal(t, routes.PathServers, path, "a rejected address stays on the picker")
	assert.Contains(t, flash, "invalid server URL")
}

func TestSelectServerDiscardsAnErrorPlexReturned(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, selectedAuth(t, errSelectFailed), silentSources(t))

	form := url.Values{
		"customUrl": {"http://10.0.0.9:32400"},
		"token":     {"form-token"},
	}.Encode()

	answer := selectServer(t, handler, form)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, answer.header.Get(fiber.HeaderLocation),
		"the picker is gone, so the browser lands on the dashboard and the "+
			"warning log is all that is left of the failure")
}

func TestBindSelectedURLPrefersTheFormToken(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Select(mock.Anything, mock.Anything).Return(nil)

	handler := pageHandler(t, auth, silentSources(t))

	app := sessionAppWithRoutes(t, handler, serversRoutes)

	seeded := seedTokenFor(t, app, "session-token")

	form := url.Values{
		"token":     {"form-token"},
		"customUrl": {"http://10.0.0.9:32400"},
	}.Encode()

	answer := serveWithCookies(t, app, seeded, "/bind", form)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, answer.header.Get(fiber.HeaderLocation))
}

func TestBindSelectedURLFallsBackToTheSessionToken(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().
		Select(mock.Anything, mock.MatchedBy(func(server plex.Server) bool {
			return server.Token == "session-token"
		})).
		Return(nil)

	handler := pageHandler(t, auth, silentSources(t))

	app := sessionAppWithRoutes(t, handler, serversRoutes)

	seeded := seedTokenFor(t, app, "session-token")

	form := url.Values{"customUrl": {"http://10.0.0.9:32400"}}.Encode()

	answer := serveWithCookies(t, app, seeded, "/bind", form)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
}

func TestBindSelectedURLRejectsAFormWithNoServer(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, offlineAuth(t), silentSources(t))

	app := sessionAppWithRoutes(t, handler, serversRoutes)

	seeded := seedTokenFor(t, app, "session-token")

	answer := serveWithCookies(t, app, seeded, "/bind",
		url.Values{"token": {"form-token"}}.Encode())

	require.Equal(t, fiber.StatusSeeOther, answer.status)

	path, flash := flashOf(t, answer.header.Get(fiber.HeaderLocation))
	assert.Equal(t, routes.PathServers, path)
	assert.Contains(t, flash, "invalid server URL")
}

func TestSelectedServerSplitsTheCustomURL(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, offlineAuth(t), silentSources(t))

	server, ok := selectedServerOf(t, handler, url.Values{
		"customUrl": {"https://plex.example.com:1234/base"},
		"token":     {"form-token"},
	})

	require.True(t, ok)
	assert.Equal(t, "plex.example.com", server.Address)
	assert.Equal(t, 1234, server.Port)
	assert.Equal(t, "https", server.Scheme)
	assert.Equal(t, "form-token", server.Token)
}

func TestSelectedServerReadsTheServerParts(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, offlineAuth(t), silentSources(t))

	server, ok := selectedServerOf(t, handler, url.Values{
		"scheme":  {"http"},
		"address": {"10.0.0.5"},
		"port":    {"32400"},
		"token":   {"form-token"},
	})

	require.True(t, ok)
	assert.Equal(t, "10.0.0.5", server.Address)
	assert.Equal(t, 32400, server.Port)
	assert.Equal(t, "http", server.Scheme)
	assert.Equal(t, "form-token", server.Token)
}

func TestSelectedServerRejectsAnEmptyForm(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, offlineAuth(t), silentSources(t))

	server, ok := selectedServerOf(t, handler, url.Values{})

	assert.False(t, ok)
	assert.Equal(t, plex.Server{}, server)
}

func TestSelectedServerRejectsACustomURLWithoutAnAddress(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, offlineAuth(t), silentSources(t))

	server, ok := selectedServerOf(t, handler,
		url.Values{"customUrl": {"http://"}})

	assert.False(t, ok)
	assert.Equal(t, plex.Server{}, server)
}

// selectedServerOf reads the server a form names, which needs a live request.
//
// Parameters:
//   - t: The test the read belongs to.
//   - handler: The handler under test.
//   - form: Form values the picker posted.
//
// Returns:
//   - server: The server the form names.
//   - ok: Whether the form named one that can be built.
func selectedServerOf(
	t *testing.T,
	handler *Handler,
	form url.Values,
) (plex.Server, bool) {
	t.Helper()

	var (
		server plex.Server
		ok     bool
	)

	app := fiber.New()
	app.Get(routes.PathServers, func(ctx fiber.Ctx) error {
		server, ok = handler.selectedServer(ctx, ctx.FormValue("token"))

		return ctx.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(requestWithCookies(
		t, http.MethodGet, routes.PathServers, form.Encode(), []*http.Cookie{},
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	return server, ok
}

func TestDiscoverServersOffersNothingWithoutASessionToken(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, discoveredAuth(t, nil), silentSources(t))

	app := sessionAppWithRoutes(t, handler, serversRoutes)

	cookies := seedTokenFor(t, app, "")

	assert.Empty(t, discoverWithCookies(t, app, cookies),
		"without a token there is nothing to look up")
}

func TestDiscoverServersDiscardsAnAccountItCannotList(t *testing.T) {
	t.Parallel()

	handler := pageHandler(t, discoveredAuth(t, errListFailed), silentSources(t))

	app := sessionAppWithRoutes(t, handler, serversRoutes)

	seeded := seedTokenFor(t, app, "session-token")

	servers := discoverWithCookies(t, app, seeded)

	assert.Empty(t, servers,
		"an account that cannot be listed offers nothing rather than a partial list")
}

// discoverWithCookies asks the app what it would offer a browser holding the
// given session cookie.
//
// Parameters:
//   - t: The test the request belongs to.
//   - app: The app to serve.
//   - cookies: Session cookie to send.
//
// Returns:
//   - servers: The servers the discover route reported.
func discoverWithCookies(
	t *testing.T,
	app *fiber.App,
	cookies []*http.Cookie,
) []plex.Server {
	t.Helper()

	resp, err := app.Test(requestWithCookies(
		t, http.MethodGet, "/discover", "", cookies,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	var servers []plex.Server

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&servers))

	return servers
}
