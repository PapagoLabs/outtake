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
