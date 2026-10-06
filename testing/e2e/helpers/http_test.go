// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package helpers

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/app"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// TestCSRFHandshakeIsRequired pins the assumption the harness request helpers are
// built on: the CSRF middleware refuses an unsafe request that carries no token,
// and accepts the same request once the token rides in both the CSRF header and
// the CSRF cookie. Because Application.Test keeps no cookie jar, the harness has
// to mint a token per request and echo it in both places.
func TestCSRFHandshakeIsRequired(t *testing.T) {
	t.Parallel()

	application := newProbeApp(t)
	ctx := context.Background()

	postStatus := func(header, cookie string) int {
		req, reqErr := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"http://127.0.0.1:8080"+ClipsPath,
			http.NoBody,
		)
		require.NoError(t, reqErr)

		req.Header.Set(csrf.HeaderName, header)

		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: csrf.ConfigDefault.CookieName, Value: cookie})
		}

		resp, doErr := application.Test(req)
		require.NoError(t, doErr)

		defer func() { _ = resp.Body.Close() }()

		_, _ = io.Copy(io.Discard, resp.Body)

		return resp.StatusCode
	}

	token := csrfTokenFor(t, application)

	assert.Equal(
		t,
		http.StatusForbidden,
		postStatus(token, ""),
		"a POST that carries no CSRF cookie is refused",
	)

	assert.Equal(
		t,
		http.StatusForbidden,
		postStatus(token, csrfTokenFor(t, application)),
		"a POST whose cookie does not match its header is refused",
	)

	assert.NotEqual(
		t,
		http.StatusForbidden,
		postStatus(token, token),
		"the same POST passes once one token rides in the header and the cookie",
	)
}

// TestSafeRequestPublishesACSRFCookie covers the other half of the handshake:
// the safe request the harness mints tokens from does publish one.
func TestSafeRequestPublishesACSRFCookie(t *testing.T) {
	t.Parallel()

	application := newProbeApp(t)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://127.0.0.1:8080"+HealthPath,
		http.NoBody,
	)
	require.NoError(t, err)

	resp, err := application.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, csrfTokenFor(t, application))
}

// newProbeApp builds a throwaway application for the CSRF probe.
//
// The listen address only has to be well formed: the probe requests go through
// the composition root's Test entry point, so nothing binds the port.
func newProbeApp(t *testing.T) *app.App {
	t.Helper()

	dir := t.TempDir()

	cfg := &config.Config{
		ListenAddr:            "127.0.0.1:8080",
		DatabasePath:          filepath.Join(dir, "outtake.db"),
		StoragePath:           filepath.Join(dir, "storage"),
		FFmpegPath:            ResolveBinary(ffmpegBin),
		FFprobePath:           ResolveBinary(ffprobeBin),
		LogLevel:              "error",
		Env:                   Environment,
		SessionPoll:           sessionPollSeconds * time.Second,
		NumWorkers:            numWorkers,
		MaxConcurrentPreviews: maxConcurrentPreviews,
		MaxClipDur:            maxClipDuration,
		PlexClientID:          ClientID,
		// httptest requests name example.com as their host.
		AllowedHosts: "example.com",
	}

	application, err := app.New(cfg)
	require.NoError(t, err)

	t.Cleanup(application.Close)

	return application
}

// csrfTokenFor mints a CSRF token from a safe request against the probe app.
func csrfTokenFor(t *testing.T, application *app.App) string {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://127.0.0.1:8080"+HealthPath,
		http.NoBody,
	)
	require.NoError(t, err)

	resp, err := application.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	for _, cookie := range resp.Cookies() {
		if cookie.Name == csrf.ConfigDefault.CookieName && cookie.Value != "" {
			return cookie.Value
		}
	}

	require.FailNow(t, "the safe request published no CSRF cookie")

	return ""
}
