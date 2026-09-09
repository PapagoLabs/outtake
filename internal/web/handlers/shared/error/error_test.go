// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package error

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"
)

func TestPageErrorRendersHTMLNotFound(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{ErrorHandler: PageError})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/missing-page", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assert.Contains(t, string(body), "Page not found")
	assert.NotContains(t, string(body), "request logger")
}

func TestPageErrorHTMXFlash(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{ErrorHandler: PageError})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/missing-page", nil)
	req.Header.Set("Hx-Request", "true")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assert.Contains(t, string(body), `<hx-partial hx-target="#flash">`)
	assert.Contains(t, string(body), "That page does not exist.")
	assert.NotContains(t, string(body), "Page not found")
}

func TestPageErrorJSONForAPI(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{ErrorHandler: PageError})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/missing", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assert.Contains(t, string(body), `"error":"http_error"`)
}

func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()

	err := resp.Body.Close()
	require.NoError(t, err)
}
