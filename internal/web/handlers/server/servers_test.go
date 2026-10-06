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
	"github.com/PapagoLabs/outtake/internal/plex/identity"
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
// boundServer is the connection a successful choice resolves to.
var boundServer = plex.Server{
	Name:      "Home Plex",
	Address:   "10.0.0.9",
	Port:      32400,
	Token:     "device-token",
	Scheme:    "http",
	Local:     true,
	MachineID: "machine-1",
	Relay:     false,
}

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
		false, form)
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
	assertBodyContains(t, answer.body, `name="server"`,
		"the form posts the connection's selection key")
	assertBodyOmits(t, answer.body, "page-token",
		"the page never carries a Plex token")
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

func TestServersOffersToForgetTheServerInUse(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Selected().Return(plex.Server{Name: "Attic"}, true).Maybe()

	answer := getServers(t, pageHandler(t, auth, silentSources(t)))

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `action="/servers/forget"`,
		"a server in use can be forgotten")
	assertBodyContains(t, answer.body, "data-confirm",
		"forgetting the server asks first")
}

func TestServersOffersNothingToForgetWithoutAServerInUse(t *testing.T) {
	t.Parallel()

	answer := getServers(t, pageHandler(t, offlineAuth(t), silentSources(t)))

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyOmits(t, answer.body, "/servers/forget",
		"there is no server to forget")
}

func TestForgetServerReturnsToTheServerPicker(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().ForgetServer(mock.Anything).Return(nil).Once()

	app := fiber.New()
	app.Post("/servers/forget", pageHandler(t, auth, silentSources(t)).ForgetServer)

	answer := serveMethod(t, app, http.MethodPost, "/servers/forget", false, "")

	path, flash := flashOf(t, answer.header.Get(fiber.HeaderLocation))
	assert.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathServers, path, "the owner picks the next server")
	assert.Empty(t, flash)
}

func TestForgetServerReportsAServerItCouldNotForget(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().ForgetServer(mock.Anything).Return(errSelectFailed).Once()

	app := fiber.New()
	app.Post("/servers/forget", pageHandler(t, auth, silentSources(t)).ForgetServer)

	answer := serveMethod(t, app, http.MethodPost, "/servers/forget", false, "")

	path, flash := flashOf(t, answer.header.Get(fiber.HeaderLocation))
	assert.Equal(t, routes.PathServers, path)
	assert.NotEmpty(t, flash, "the owner is told the server was not forgotten")
}

func TestSelectServerBindsTheChosenConnection(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().ChooseServer(mock.Anything, mock.Anything, "machine-1 http://10.0.0.9:32400").
		Return(boundServer, nil).Once()
	auth.EXPECT().Select(mock.Anything, boundServer).Return(nil).Once()

	answer := selectServer(t, pageHandler(t, auth, silentSources(t)),
		url.Values{"server": {"machine-1 http://10.0.0.9:32400"}}.Encode())

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, answer.header.Get(fiber.HeaderLocation),
		"a bound install goes straight to the dashboard")
}

func TestSelectServerBindsAVerifiedCustomURL(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().ChooseCustomURL(mock.Anything, mock.Anything, "https://plex.example.com").
		Return(boundServer, nil).Once()
	auth.EXPECT().Select(mock.Anything, boundServer).Return(nil).Once()

	answer := selectServer(t, pageHandler(t, auth, silentSources(t)),
		url.Values{"customUrl": {"https://plex.example.com"}}.Encode())

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, answer.header.Get(fiber.HeaderLocation))
}

func TestSelectServerDiscoversWithTheSessionTokenAndIgnoresAPostedOne(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().ChooseCustomURL(mock.Anything, "session-token", "https://plex.example.com").
		Return(boundServer, nil).Once()
	auth.EXPECT().Select(mock.Anything, boundServer).Return(nil).Once()

	app := sessionAppWithRoutes(t, pageHandler(t, auth, silentSources(t)), serversRoutes)
	seeded := seedTokenFor(t, app, "session-token")

	form := url.Values{
		"customUrl": {"https://plex.example.com"},
		"token":     {"posted-token"},
		"address":   {"attacker.example"},
	}.Encode()

	answer := serveWithCookies(t, app, seeded, "/bind", form)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, answer.header.Get(fiber.HeaderLocation))
}

func TestSelectServerExplainsARefusedChoice(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		err  error
		want string
	}{
		"not on the account": {err: identity.ErrServerNotFound, want: "not a Plex server on your account"},
		"unreachable":        {err: identity.ErrServerUnreachable, want: "cannot reach that server"},
		"not http":           {err: identity.ErrInvalidServerURL, want: "http or https URL"},
		"anything else":      {err: errListFailed, want: "could not use that server"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Select has no expectation, so a refused choice that binds fails the test.
			auth := mocks.NewMockPlexAuth(t)
			auth.EXPECT().ChooseCustomURL(mock.Anything, mock.Anything, mock.Anything).
				Return(plex.EmptyServer(), test.err).Once()

			answer := selectServer(t, pageHandler(t, auth, silentSources(t)),
				url.Values{"customUrl": {"https://plex.example.com"}}.Encode())

			path, flash := flashOf(t, answer.header.Get(fiber.HeaderLocation))
			assert.Equal(t, routes.PathServers, path, "a refused choice stays on the picker")
			assert.Contains(t, flash, test.want)
		})
	}
}

func TestSelectServerStillLandsOnTheDashboardWhenTheChoiceCannotBeSaved(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().
		ChooseServer(mock.Anything, mock.Anything, mock.Anything).
		Return(boundServer, nil).
		Once()
	auth.EXPECT().Select(mock.Anything, boundServer).Return(errSelectFailed).Once()

	answer := selectServer(t, pageHandler(t, auth, silentSources(t)),
		url.Values{"server": {"machine-1 http://10.0.0.9:32400"}}.Encode())

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, answer.header.Get(fiber.HeaderLocation),
		"the binding is live, so the browser lands on the dashboard and the "+
			"warning log is all that is left of the failure")
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
