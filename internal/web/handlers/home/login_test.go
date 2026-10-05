// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// loginApp mounts the login route on an app that carries a session.
//
// Parameters:
//   - t: The test the app belongs to.
//   - handler: The handler under test.
//
// Returns:
//   - app: The app the route is mounted on.
func loginApp(t *testing.T, handler *Handler) *fiber.App {
	t.Helper()

	app := fiber.New()
	app.Use(sessionMiddleware(t))
	app.Get(routes.PathLogin, handler.Login)

	return app
}

// loginHandler wires a login page around an unbound Plex installation.
//
// Parameters:
//   - t: The test the handler belongs to.
//
// Returns:
//   - handler: The handler under test.
func loginHandler(t *testing.T) *Handler {
	t.Helper()

	return New(queueForTest(t), nil, offlineAuth(t), nil, nil)
}

func TestLoginSendsAnAuthenticatedVisitorHome(t *testing.T) {
	t.Parallel()

	app := loginApp(t, loginHandler(t))

	cookies := seedTokenFor(t, app, "session-token")

	answer := serveWithCookies(t, app, cookies, routes.PathLogin, "")

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathRoot, answer.header.Get(fiber.HeaderLocation),
		"a visitor who already has a token has nothing to log in to")
}

func TestLoginRendersTheSignInPage(t *testing.T) {
	t.Parallel()

	app := loginApp(t, loginHandler(t))

	cookies := seedTokenFor(t, app, "")

	answer := serveWithCookies(t, app, cookies, routes.PathLogin, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Sign in with Plex",
		"the page has to offer the Plex sign-in a user can start")
	assertBodyContains(t, answer.body, `name="token"`,
		"a user with no plex.tv access still has to be able to paste a token")
}

func TestLoginShowsTheCarriedFailure(t *testing.T) {
	t.Parallel()

	app := loginApp(t, loginHandler(t))

	cookies := seedTokenFor(t, app, "")

	target := routes.PathLogin + "?" + url.Values{
		routes.QueryError: {"No PIN session"},
	}.Encode()

	answer := serveWithCookies(t, app, cookies, target, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "No PIN session",
		"the reason the last attempt failed is what the user has to read")
}
