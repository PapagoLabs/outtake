// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/identity/mocks"
)

// errStoreUnreadable reports a token store that cannot be read.
var errStoreUnreadable = errors.New("token store is unreadable")

func TestAuthGuardPassesTheE2EEnvironmentThroughWithoutAToken(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, guardApp(t, e2eEnv, nil), http.MethodGet, "/dashboard/sessions")

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

func TestAuthGuardPassesThroughARequestCarryingAToken(t *testing.T) {
	t.Parallel()

	app := guardApp(t, "test", withToken("session-token"))

	for _, target := range []string{"/api/clips", "/dashboard/sessions"} {
		got := issueRequest(t, app, http.MethodGet, target)

		assert.Equal(t, fiber.StatusOK, got.status, target)
	}
}

func TestRestoreTokenPassesThroughWhenThereIsNoSession(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)

	reached := false

	app := chainApp(t, func(ctx fiber.Ctx) error {
		reached = true

		assert.Nil(t, session.FromContext(ctx))

		return ctx.SendStatus(fiber.StatusNoContent)
	}, RestoreToken(store))

	got := issueRequest(t, app, http.MethodGet, "/dashboard/sessions")

	assert.Equal(t, fiber.StatusNoContent, got.status)
	assert.True(t, reached, "the chain still runs without a session")
	store.AssertNotCalled(t, "LatestToken", mock.Anything)
}

func TestRestoreTokenKeepsAnExistingTokenWithoutReadingTheStore(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)

	app := chainApp(t, func(ctx fiber.Ctx) error {
		return ctx.SendString(identity.Token(session.FromContext(ctx)))
	}, session.New(), withToken("session-token"), RestoreToken(store))

	got := issueRequest(t, app, http.MethodGet, "/dashboard/sessions")

	assert.Equal(t, "session-token", got.body)
	store.AssertNotCalled(t, "LatestToken", mock.Anything)
}

func TestRestoreTokenSeedsAnEmptySessionFromTheStore(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		LatestToken(mock.Anything).
		Return("restored-token", nil).
		Once()

	app := chainApp(t, func(ctx fiber.Ctx) error {
		return ctx.SendString(identity.Token(session.FromContext(ctx)))
	}, session.New(), RestoreToken(store))

	got := issueRequest(t, app, http.MethodGet, "/dashboard/sessions")

	assert.Equal(t, "restored-token", got.body)
	store.AssertExpectations(t)
}

func TestRestoreTokenLeavesTheTokenUnsetWhenTheStoreFails(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		LatestToken(mock.Anything).
		Return("", errStoreUnreadable).
		Once()

	app := chainApp(t, func(ctx fiber.Ctx) error {
		return ctx.SendString(identity.Token(session.FromContext(ctx)))
	}, session.New(), RestoreToken(store))

	got := issueRequest(t, app, http.MethodGet, "/dashboard/sessions")

	assert.Empty(t, got.body)
	store.AssertExpectations(t)
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
// the terminal handler.
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

	return chainApp(t, okHandler(), append(middle, AuthGuard(env))...)
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
