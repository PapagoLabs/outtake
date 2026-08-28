// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	fiber "github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/api"
)

const formContentType = "application/x-www-form-urlencoded"

func TestWriteErrorJSON(t *testing.T) {
	t.Parallel()

	resp := postWriteError(t, fiber.MIMEApplicationJSON, `{}`, "")
	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var payload api.ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	assert.Equal(t, "invalid_duration", payload.Error)
	assert.Equal(t, "must be between 0 and 600 seconds", payload.Message)
}

func TestWriteErrorFormRedirectsToMediaItem(t *testing.T) {
	t.Parallel()

	resp := postWriteError(t, formContentType, "mediaId=42&duration=0", "")
	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusFound, resp.StatusCode)
	assertFormErrorLocation(t, resp, "/media/item/42", "must be between 0 and 600 seconds")
}

func TestWriteErrorFormRedirectsToReferer(t *testing.T) {
	t.Parallel()

	resp := postWriteError(
		t,
		formContentType,
		"token=",
		"https://evil.example/login?next=1",
	)
	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusFound, resp.StatusCode)
	assertFormErrorLocation(t, resp, "/login", "must be between 0 and 600 seconds")
}

func TestPathWithErrorEncodesSpaces(t *testing.T) {
	t.Parallel()

	location := pathWithError(pathLogin, "No PIN session")
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	assert.Equal(t, pathLogin, parsed.Path)
	assert.Equal(t, "No PIN session", parsed.Query().Get(queryError))
	assert.NotContains(t, location, " ")
}

func TestRefererPathStripsHost(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/login", refererPath("https://evil.example/login"))
	assert.Equal(t, "/media?q=1", refererPath("http://host/media?q=1"))
	assert.Equal(t, pathRoot, refererPath("https://evil.example"))
}

func postWriteError(t *testing.T, contentType, body, referer string) *http.Response {
	t.Helper()

	app := fiber.New()
	app.Post("/err", func(ctx fiber.Ctx) error {
		return writeError(
			ctx,
			fiber.StatusBadRequest,
			"invalid_duration",
			"must be between 0 and 600 seconds",
		)
	})

	req := httptest.NewRequest(http.MethodPost, "/err", strings.NewReader(body))
	req = req.WithContext(t.Context())
	req.Header.Set(fiber.HeaderContentType, contentType)
	if referer != "" {
		req.Header.Set(fiber.HeaderReferer, referer)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	return resp
}

func assertFormErrorLocation(t *testing.T, resp *http.Response, wantPath, wantError string) {
	t.Helper()

	parsed, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, wantPath, parsed.Path)
	assert.Equal(t, wantError, parsed.Query().Get(queryError))
	assert.Empty(t, parsed.Host)
}

func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()

	err := resp.Body.Close()
	require.NoError(t, err)
}
