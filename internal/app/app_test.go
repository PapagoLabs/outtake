// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

func TestNew_DefaultBackends(t *testing.T) {
	t.Parallel()

	application, err := New(testAppConfig(t, ""))
	require.NoError(t, err)
	application.Close()
}

func TestSecurityHeadersAndCSRF(t *testing.T) {
	t.Parallel()

	application, err := New(testAppConfig(t, ""))
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
}

func TestHTMXErrorsCarryTheFlashPartial(t *testing.T) {
	t.Parallel()

	application, err := New(testAppConfig(t, ""))
	require.NoError(t, err)

	defer application.Close()

	htmxReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/login", nil)
	htmxReq.Header.Set("Hx-Request", "true")

	htmxResp, err := application.router.Test(htmxReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, htmxResp.Body.Close())
	}()

	htmxBody, err := io.ReadAll(htmxResp.Body)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusForbidden, htmxResp.StatusCode, "the status stays truthful")
	assert.Equal(t, "text/html; charset=utf-8", htmxResp.Header.Get(fiber.HeaderContentType))
	assert.Contains(t, string(htmxBody), `<template hx type="partial" hx-target="#flash">`,
		"htmx is handed the partial it routes to #flash")
	assert.Equal(t, "none", htmxResp.Header.Get("Hx-Reswap"),
		"an error must not swap the element that issued the request")
	assert.Contains(t, string(htmxBody), "invalid csrf token",
		"the partial carries the reason")
	assert.NotContains(t, string(htmxBody), `"http_error"`,
		"an htmx request is not answered with the API error envelope")

	apiReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/login", nil)

	apiResp, err := application.router.Test(apiReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, apiResp.Body.Close())
	}()

	apiBody, err := io.ReadAll(apiResp.Body)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusForbidden, apiResp.StatusCode,
		"the representation changes, not the status")
	assert.Contains(t, string(apiBody), `"http_error"`,
		"an API caller still gets the JSON envelope")
}

func TestCSRFAllowsHTTPSOriginBehindHTTP(t *testing.T) {
	t.Parallel()

	application, err := New(testAppConfig(t, "https://clips.example"))
	require.NoError(t, err)

	defer application.Close()

	loginReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", nil)

	loginReq.Host = "clips.example"

	loginResp, err := application.router.Test(loginReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, loginResp.Body.Close())
	}()

	loginBody, err := io.ReadAll(loginResp.Body)
	require.NoError(t, err)

	token := csrfTokenFromLoginHTML(string(loginBody))
	require.NotEmpty(t, token)

	postResp := csrfOriginPost(t, application, loginResp, "https://clips.example", token)

	defer func() {
		require.NoError(t, postResp.Body.Close())
	}()

	body, err := io.ReadAll(postResp.Body)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, postResp.StatusCode, string(body))
	assert.NotContains(t, string(body), "invalid csrf token")

	evilResp := csrfOriginPost(t, application, loginResp, "https://evil.example", token)

	defer func() {
		require.NoError(t, evilResp.Body.Close())
	}()

	evilBody, err := io.ReadAll(evilResp.Body)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, evilResp.StatusCode)
	assert.Contains(t, string(evilBody), "invalid csrf token")
}

func TestLogoutRouteRequiresPost(t *testing.T) {
	t.Parallel()

	application, err := New(testAppConfig(t, ""))
	require.NoError(t, err)

	defer application.Close()

	getReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/auth/logout", nil)

	getResp, err := application.router.Test(getReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, getResp.Body.Close())
	}()

	assert.Equal(t, fiber.StatusMethodNotAllowed, getResp.StatusCode)

	postReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/logout", nil)

	postResp, err := application.router.Test(postReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, postResp.Body.Close())
	}()

	assert.Equal(t, fiber.StatusForbidden, postResp.StatusCode)
}

func TestLogoutFormFlow(t *testing.T) {
	t.Parallel()

	application, err := New(testAppConfig(t, ""))
	require.NoError(t, err)

	defer application.Close()

	loginReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", nil)

	loginResp, err := application.router.Test(loginReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, loginResp.Body.Close())
	}()

	loginBody, err := io.ReadAll(loginResp.Body)
	require.NoError(t, err)

	token := csrfTokenFromLoginHTML(string(loginBody))
	require.NotEmpty(t, token)

	err = application.db.SaveToken(t.Context(), "test-client", "plex-token")
	require.NoError(t, err)

	form := url.Values{}
	form.Set(identity.CSRFFormField, token)

	postReq := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/auth/logout",
		strings.NewReader(form.Encode()),
	)
	postReq.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	for _, cookie := range loginResp.Cookies() {
		postReq.AddCookie(cookie)
	}

	postResp, err := application.router.Test(postReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, postResp.Body.Close())
	}()

	assert.Equal(t, fiber.StatusSeeOther, postResp.StatusCode)
	assert.Equal(t, "/login", postResp.Header.Get("Location"))
}

func testAppConfig(t *testing.T, publicBaseURL string) *config.Config {
	t.Helper()

	dir := t.TempDir()

	return &config.Config{
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
		SessionPoll:     10 * time.Second,
		NumWorkers:      1,
		MaxClipDur:      600 * time.Second,
		CropBlackBars:   false,
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   publicBaseURL,
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}
}

func TestAppTestProcessesARequest(t *testing.T) {
	t.Parallel()

	application, err := New(testAppConfig(t, ""))
	require.NoError(t, err)

	defer application.Close()

	loginReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", nil)

	resp, err := application.Test(loginReq)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, resp.Body.Close())
	}()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestNewReportsAnUnknownDatabaseBackend(t *testing.T) {
	t.Parallel()

	cfg := testAppConfig(t, "")

	cfg.DatabaseBackend = "mysql"

	application, err := New(cfg)
	require.ErrorContains(t, err, "init database")
	assert.Nil(t, application)
}

func TestNewReportsAnUnknownStorageBackend(t *testing.T) {
	t.Parallel()

	cfg := testAppConfig(t, "")

	cfg.StorageBackend = "gcs"

	application, err := New(cfg)
	require.ErrorContains(t, err, "init storage")
	assert.Nil(t, application)
}

func TestPlexIdentityGeneratesAClientIDWhenNoneIsConfigured(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

	auth, bind := plexIdentity(testAppConfig(t, ""), db)
	t.Cleanup(bind.Stop)

	require.NotNil(t, auth, "a generated client id still yields an auth service")
	assert.False(t, auth.Bound(), "nothing is bound until a server is selected")

	_, ok := bind.Get()
	assert.False(t, ok)
}

func TestPlexIdentityKeepsTheConfiguredClientID(t *testing.T) {
	t.Parallel()

	cfg := testAppConfig(t, "")

	cfg.PlexClientID = "configured-client-id"

	auth, bind := plexIdentity(cfg, testDatabase(t))
	t.Cleanup(bind.Stop)

	require.NotNil(t, auth, "a configured client id still yields an auth service")
	assert.False(t, auth.Bound())
}

func TestRunReportsAListenFailure(t *testing.T) {
	t.Parallel()

	// The context is never canceled, so the shutdown goroutine Run starts stays
	// parked instead of logging after this test has finished.
	runCtx, stop := context.WithCancel(context.WithoutCancel(t.Context()))

	application := &App{
		cfg:    &config.Config{ListenAddr: "127.0.0.1:-1"},
		router: fiber.New(),
		ctx:    runCtx,
		stop:   stop,
	}

	err := application.Run()
	require.ErrorContains(t, err, "listen")
	assert.NotErrorIs(t, err, context.Canceled,
		"a bad address is a listen failure, not a shutdown")
}

func TestRunStopsWhenTheApplicationContextIsCanceled(t *testing.T) {
	t.Parallel()

	cfg := testAppConfig(t, "")

	cfg.ListenAddr = freeLoopbackAddr(t)

	application, err := New(cfg)
	require.NoError(t, err)

	defer application.Close()

	stopped := make(chan error, 1)

	go func() {
		stopped <- application.Run()
	}()

	require.Eventually(t, func() bool {
		return serving(t.Context(), cfg.ListenAddr)
	}, 10*time.Second, 5*time.Millisecond, "the server was accepting requests")

	application.stop()

	select {
	case err = <-stopped:
		require.NoError(t, err, "a shutdown the app asked for is not a failure")
	case <-time.After(10 * time.Second):
		require.Fail(t, "Run did not return after the context was canceled")
	}
}

// freeLoopbackAddr reserves and releases a loopback address so the server can
// bind it.
//
// Parameters:
//   - t: The test that needs the address.
//
// Returns:
//   - addr: A loopback address nothing is listening on.
func freeLoopbackAddr(t *testing.T) string {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	return addr
}

// serving reports whether an address answers an HTTP request. Any status counts,
// because a 404 still proves the listener reached the router.
//
// Parameters:
//   - addr: The loopback address to probe.
//
// Returns:
//   - ok: True when the server answered.
func serving(ctx context.Context, addr string) bool {
	client := &http.Client{Timeout: 5 * time.Millisecond}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://"+addr+"/api/healthz",
		http.NoBody,
	)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}

	return resp.Body.Close() == nil
}

func csrfOriginPost(
	t *testing.T,
	application *App,
	loginResp *http.Response,
	origin, token string,
) *http.Response {
	t.Helper()

	post := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/missing", nil)

	post.Host = "clips.example"
	post.Header.Set("Origin", origin)
	post.Header.Set("X-Csrf-Token", token)

	for _, cookie := range loginResp.Cookies() {
		post.AddCookie(cookie)
	}

	resp, err := application.router.Test(post)
	require.NoError(t, err)

	return resp
}

func csrfTokenFromLoginHTML(body string) string {
	const prefix = `name="csrf-token" content="`

	_, after, found := strings.Cut(body, prefix)
	if !found {
		return ""
	}

	token, _, found := strings.Cut(after, `"`)
	if !found {
		return ""
	}

	return token
}
