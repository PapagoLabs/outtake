// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package helpers

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/app"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// sessionCookieName is the cookie the web layer keeps the session id in.
const sessionCookieName = "session_id"

// TestCSRFHandshakeIsRequired pins the assumption the harness request helpers
// are built on: CSRF tokens live in the session, so the middleware refuses an
// unsafe request unless it carries the session that minted the token in its
// CSRF header. Because Application.Test keeps no cookie jar, the harness opens
// a session per unsafe request and echoes its cookies and token.
func TestCSRFHandshakeIsRequired(t *testing.T) {
	t.Parallel()

	application := newProbeApp(t)
	ctx := context.Background()

	postStatus := func(token string, cookies []*http.Cookie) int {
		req, reqErr := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"http://127.0.0.1:8080"+ClipsPath,
			http.NoBody,
		)
		require.NoError(t, reqErr)

		req.Header.Set(csrf.HeaderName, token)

		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}

		resp, doErr := application.Test(req)
		require.NoError(t, doErr)

		defer func() { _ = resp.Body.Close() }()

		_, _ = io.Copy(io.Discard, resp.Body)

		return resp.StatusCode
	}

	token, cookies := handshakeFor(t, application)
	_, otherCookies := handshakeFor(t, application)

	assert.Equal(
		t,
		http.StatusForbidden,
		postStatus(token, nil),
		"a POST that carries no session is refused",
	)

	assert.Equal(
		t,
		http.StatusForbidden,
		postStatus(token, otherCookies),
		"a POST whose token another session minted is refused",
	)

	assert.NotEqual(
		t,
		http.StatusForbidden,
		postStatus(token, cookies),
		"the same POST passes with the session that minted its token",
	)
}

// TestHandshakeRequestOpensASession covers the other half of the handshake:
// the safe request the harness mints tokens from opens a session and publishes
// its token, which the health endpoint, mounted before the session middleware,
// never does.
func TestHandshakeRequestOpensASession(t *testing.T) {
	t.Parallel()

	application := newProbeApp(t)

	token, cookies := handshakeFor(t, application)

	assert.NotEmpty(t, token)
	assert.True(t, slices.ContainsFunc(cookies, func(cookie *http.Cookie) bool {
		return cookie.Name == sessionCookieName && cookie.Value != ""
	}), "the handshake request opens a session")
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

// handshakeFor opens a session on the probe app with the harness's handshake
// request, and returns the CSRF token it minted with the cookies it set.
func handshakeFor(t *testing.T, application *app.App) (string, []*http.Cookie) {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://127.0.0.1:8080"+HandshakePath,
		http.NoBody,
	)
	require.NoError(t, err)

	resp, err := application.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	cookies := resp.Cookies()

	for _, cookie := range cookies {
		if cookie.Name == csrf.ConfigDefault.CookieName && cookie.Value != "" {
			return cookie.Value, cookies
		}
	}

	require.FailNow(t, "the handshake request published no CSRF cookie")

	return "", nil
}
