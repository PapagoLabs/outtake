// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/identity/mocks"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// errStoreClosed reports a store that cannot be written to.
var errStoreClosed = errors.New("database is closed")

// testAuth builds the handler under test around a Plex authentication service.
//
// Parameters:
//   - store: Token store the service persists through, which may be nil.
//
// Returns:
//   - handler: The auth handler under test.
func testAuth(store identity.TokenStore) *Handler {
	return New(identity.New("outtake", "test-client", "http://localhost", store, nil))
}

func TestAuthLoginFormWithoutTokenRedirects(t *testing.T) {
	t.Parallel()

	handler := testAuth(nil)
	app := fiber.New()
	app.Post("/api/auth/login", handler.Login)

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/auth/login",
		strings.NewReader("token="),
	)
	req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)

	parsed, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, routes.PathLogin, parsed.Path)
	assert.Equal(t, msgPlexTokenRequired, parsed.Query().Get(routes.QueryError))
}

func TestAuthLogoutRedirectsToLogin(t *testing.T) {
	t.Parallel()

	handler := testAuth(nil)
	app := fiber.New()
	app.Post("/api/auth/logout", handler.Logout)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/logout", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, routes.PathLogin, resp.Header.Get("Location"))
}

func TestAuthLogoutRejectsGet(t *testing.T) {
	t.Parallel()

	handler := testAuth(nil)
	app := fiber.New()
	app.Post("/api/auth/logout", handler.Logout)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/auth/logout", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusMethodNotAllowed, resp.StatusCode)
}

func TestAuthLogoutFailsWhenClearAuthFails(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		ClearAuth(mock.Anything).
		Return(errStoreClosed)

	handler := testAuth(store)
	app := fiber.New()
	app.Post("/api/auth/logout", handler.Logout)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/logout", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
}

// closeBody closes a response body and fails the test when it cannot.
//
// Parameters:
//   - t: The test the response belongs to.
//   - resp: The response to close.
func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()

	require.NoError(t, resp.Body.Close())
}
