// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/config"
)

func TestNew_DefaultBackends(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{
		ListenAddr:      "127.0.0.1:0",
		DatabasePath:    filepath.Join(dir, "outtake.db"),
		DatabaseBackend: "sqlite",
		DatabaseURL:     "",
		StoragePath:     filepath.Join(dir, "output"),
		StorageBackend:  "filesystem",
		S3Endpoint:      "",
		S3Bucket:        "",
		S3Region:        "",
		S3AccessKey:     "",
		S3SecretKey:     "",
		S3UsePathStyle:  true,
		FFmpegPath:      "ffmpeg",
		FFprobePath:     "ffprobe",
		LogLevel:        "info",
		Env:             "test",
		SessionPollSec:  10,
		NumWorkers:      1,
		MaxClipDurSec:   600,
		CropBlackBars:   false,
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   "",
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}

	application, err := New(cfg)
	require.NoError(t, err)
	application.Close()
}

func TestSecurityHeadersAndCSRF(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{
		ListenAddr:      "127.0.0.1:0",
		DatabasePath:    filepath.Join(dir, "outtake.db"),
		DatabaseBackend: "sqlite",
		DatabaseURL:     "",
		StoragePath:     filepath.Join(dir, "output"),
		StorageBackend:  "filesystem",
		S3Endpoint:      "",
		S3Bucket:        "",
		S3Region:        "",
		S3AccessKey:     "",
		S3SecretKey:     "",
		S3UsePathStyle:  true,
		FFmpegPath:      "ffmpeg",
		FFprobePath:     "ffprobe",
		LogLevel:        "info",
		Env:             "test",
		SessionPollSec:  10,
		NumWorkers:      1,
		MaxClipDurSec:   600,
		CropBlackBars:   false,
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   "",
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}

	application, err := New(cfg)
	require.NoError(t, err)

	defer application.Close()

	loginReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", nil)
	loginResp, err := application.router.Test(loginReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, loginResp.Body.Close())
	}()

	csp := loginResp.Header.Get("Content-Security-Policy")
	assert.Contains(t, csp, "script-src 'self'")
	assert.Contains(t, csp, "frame-ancestors 'none'")
	assert.NotContains(t, csp, "unsafe-eval")
	assert.Equal(t, "DENY", loginResp.Header.Get("X-Frame-Options"))

	loginBody, err := io.ReadAll(loginResp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(loginBody), "X-Csrf-Token")
	assert.Contains(t, string(loginBody), `name="_csrf"`)

	htmxReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/login", nil)
	htmxReq.Header.Set("Hx-Request", "true")

	htmxResp, err := application.router.Test(htmxReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, htmxResp.Body.Close())
	}()

	htmxBody, err := io.ReadAll(htmxResp.Body)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, htmxResp.StatusCode)
	assert.Contains(t, string(htmxBody), `<hx-partial hx-target="#flash">`)
}
