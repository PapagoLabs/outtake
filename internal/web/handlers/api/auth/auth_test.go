// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	fiber "github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
)

const formContentType = "application/x-www-form-urlencoded"

func TestAuthLoginFormWithoutTokenRedirects(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler("outtake", "test-client", "http://localhost", nil, nil)
	app := fiber.New()
	app.Post("/api/auth/login", handler.Login)

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/auth/login",
		strings.NewReader("token="),
	)
	req.Header.Set(fiber.HeaderContentType, formContentType)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)

	parsed, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, respond.PathLogin, parsed.Path)
	assert.Equal(t, msgPlexTokenRequired, parsed.Query().Get(respond.QueryError))
}

func TestAuthLogoutRedirectsToLogin(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler("outtake", "test-client", "http://localhost", nil, nil)
	app := fiber.New()
	app.Get("/api/auth/logout", handler.Logout)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/auth/logout", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, respond.PathLogin, resp.Header.Get("Location"))
}

func TestAuthLogoutFailsWhenClearAuthFails(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/logout.db")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	handler := NewAuthHandler("outtake", "test-client", "http://localhost", db, nil)
	app := fiber.New()
	app.Get("/api/auth/logout", handler.Logout)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/auth/logout", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
}

// closeBody closes resp.Body and ignores the error.
//
// Parameters:
//   - t: Test harness. Callee should call t.Helper when wrapping.
//   - resp: HTTP response to validate or close.
func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}
