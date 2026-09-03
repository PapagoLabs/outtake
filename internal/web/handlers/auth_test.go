// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/database"
)

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
	assert.Equal(t, pathLogin, parsed.Path)
	assert.Equal(t, msgPlexTokenRequired, parsed.Query().Get(queryError))
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
	assert.Equal(t, pathLogin, resp.Header.Get("Location"))
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
