// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/web/middleware/mocks"
)

// errStoreUnreadable reports a user store that cannot be read.
var errStoreUnreadable = errors.New("user store is unreadable")

func TestAuthGuardPassesTheE2EEnvironmentThroughWithoutAToken(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, guardApp(t, config.EnvE2E, nil), http.MethodGet, "/dashboard/sessions")

	assert.Equal(t, fiber.StatusOK, got.status)
}

func TestAuthGuardAnswersAnonymousAPIRequestsWithAnErrorEnvelope(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, guardApp(t, "test", nil), http.MethodGet, "/api/clips")

	assert.Equal(t, fiber.StatusUnauthorized, got.status)
	assert.JSONEq(t, `{"error":"unauthorized","message":"authentication required"}`, got.body)
}

func TestAuthGuardRedirectsAnonymousPageRequestsToLogin(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"/dashboard/sessions", "/clips/42", "/"} {
		got := issueRequest(t, guardApp(t, "test", nil), http.MethodGet, target)

		assert.Equal(t, fiber.StatusSeeOther, got.status, target)
		assert.Equal(t, "/login", got.location, target)
	}
}

func TestAuthGuardPassesASessionSignedInAsAKnownUser(t *testing.T) {
	t.Parallel()

	users := mocks.NewMockUserLookup(t)
	users.EXPECT().UserRole(mock.Anything, 42).Return("owner", true, nil).Twice()

	app := guardAppWith(t, users, withUser(42))

	for _, target := range []string{"/api/clips", "/dashboard/sessions"} {
		got := issueRequest(t, app, http.MethodGet, target)

		assert.Equal(t, fiber.StatusOK, got.status, target)
	}
}

func TestAuthGuardRefusesATokenWithoutAUser(t *testing.T) {
	t.Parallel()

	// A lookup with no expectations fails the test if the guard consults it.
	users := mocks.NewMockUserLookup(t)

	got := issueRequest(t, guardAppWith(t, users, withToken("session-token")), http.MethodGet, "/")

	assert.Equal(t, fiber.StatusSeeOther, got.status)
	assert.Equal(t, "/login", got.location)
}

func TestAuthGuardRefusesEverySessionWithoutAUserStore(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, guardAppWith(t, nil, withUser(42)), http.MethodGet, "/api/clips")

	assert.Equal(t, fiber.StatusUnauthorized, got.status)
}

func TestAuthGuardSignsOutASessionWhoseUserWasRemoved(t *testing.T) {
	t.Parallel()

	users := mocks.NewMockUserLookup(t)
	users.EXPECT().UserRole(mock.Anything, 7).Return("", false, nil).Once()

	app := chainApp(t, func(ctx fiber.Ctx) error {
		return ctx.SendString(identity.Token(session.FromContext(ctx)))
	},
		session.New(),
		withUser(7),
		func(ctx fiber.Ctx) error {
			err := AuthGuard("test", users)(ctx)

			// The guard answered with a redirect. Expose what it left on the
			// session, so the test can tell the session was reset.
			ctx.Set("X-Remaining-Token", identity.Token(session.FromContext(ctx)))

			return err
		},
	)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get(fiber.HeaderLocation))
	assert.Empty(t, resp.Header.Get("X-Remaining-Token"), "the revoked session was reset")
}

func TestAuthGuardFailsClosedWhenTheUserCannotBeResolved(t *testing.T) {
	t.Parallel()

	users := mocks.NewMockUserLookup(t)
	users.EXPECT().UserRole(mock.Anything, 9).Return("", false, errStoreUnreadable).Once()

	got := issueRequest(t, guardAppWith(t, users, withUser(9)), http.MethodGet, "/")

	assert.Equal(t, fiber.StatusInternalServerError, got.status)
}

func TestUnauthenticatedRedirectsAPathWithoutTheAPIPrefix(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, refusingApp(t), http.MethodGet, "/clips/42")

	assert.Equal(t, fiber.StatusSeeOther, got.status)
	assert.Equal(t, "/login", got.location)
}

func TestUnauthenticatedTreatsTheBareAPIPathAsAPage(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, refusingApp(t), http.MethodGet, "/api")

	assert.Equal(t, fiber.StatusSeeOther, got.status)
	assert.Equal(t, "/login", got.location)
}

func TestUnauthenticatedDoesNotContinueTheChain(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, refusingApp(t), http.MethodGet, "/api/clips")

	assert.Equal(t, fiber.StatusUnauthorized, got.status)
	assert.NotContains(t, got.body, "no-token")
}

// guardApp builds a session-backed application with the auth guard in front of
// the terminal handler, resolving users against an empty store.
//
// Parameters:
//   - t: The test that owns the application.
//   - env: Environment the guard is configured with.
//   - seed: Middleware run before the guard, or nil for an anonymous session.
//
// Returns:
//   - app: An application whose only route is guarded.
func guardApp(t *testing.T, env string, seed fiber.Handler) *fiber.App {
	t.Helper()

	middle := []fiber.Handler{session.New()}
	if seed != nil {
		middle = append(middle, seed)
	}

	return chainApp(t, okHandler(), append(middle, AuthGuard(env, mocks.NewMockUserLookup(t)))...)
}

// guardAppWith builds a session-backed application whose auth guard resolves
// users against a given store.
//
// Parameters:
//   - t: The test that owns the application.
//   - users: User store the guard consults, which may be nil.
//   - seed: Middleware run before the guard.
//
// Returns:
//   - app: An application whose only route is guarded.
func guardAppWith(t *testing.T, users UserLookup, seed fiber.Handler) *fiber.App {
	t.Helper()

	return chainApp(t, okHandler(), session.New(), seed, AuthGuard("test", users))
}

// refusingApp builds an application that answers every request with
// unauthenticated and never reaches the handler behind it.
//
// Parameters:
//   - t: The test that owns the application.
//
// Returns:
//   - app: An application whose only handler rejects the request.
func refusingApp(t *testing.T) *fiber.App {
	t.Helper()

	return chainApp(t, writeHandler("no-token"), func(ctx fiber.Ctx) error {
		return unauthenticated(ctx)
	})
}

// withToken seeds the session with a Plex access token before the guard runs.
//
// Parameters:
//   - token: Plex access token to store on the session.
//
// Returns:
//   - handler: Middleware that seeds the session and continues the chain.
func withToken(token string) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		identity.SetToken(session.FromContext(ctx), token)

		return ctx.Next()
	}
}

// withUser seeds the session with a Plex access token and the Plex user id it
// signed in as, before the guard runs.
//
// Parameters:
//   - plexUserID: Plex user id to store on the session.
//
// Returns:
//   - handler: Middleware that seeds the session and continues the chain.
func withUser(plexUserID int) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		sess := session.FromContext(ctx)
		identity.SetToken(sess, "session-token")
		identity.SetUserID(sess, plexUserID)

		return ctx.Next()
	}
}
