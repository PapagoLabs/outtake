// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// csrfSource names where a request carries its CSRF token.
type csrfSource string

func TestCSRFCookieSecureFollowsPublicURL(t *testing.T) {
	t.Parallel()

	httpCfg := &config.Config{ListenAddr: "127.0.0.1:8080"}
	assert.False(t, csrfConfig(httpCfg, nil).CookieSecure)

	httpsCfg := &config.Config{PublicBaseURL: "https://clips.example"}
	assert.True(t, csrfConfig(httpsCfg, nil).CookieSecure)
}

func TestCSRFTrustedOriginsFollowsPublicBaseURL(t *testing.T) {
	t.Parallel()

	local := csrfConfig(&config.Config{ListenAddr: "127.0.0.1:8080"}, nil)
	assert.Empty(t, local.TrustedOrigins)

	behindTLS := csrfConfig(&config.Config{PublicBaseURL: "https://clips.example/"}, nil)
	assert.Equal(t, []string{"https://clips.example"}, behindTLS.TrustedOrigins)

	withPath := csrfConfig(&config.Config{PublicBaseURL: "https://clips.example/outtake"}, nil)
	assert.Equal(t, []string{"https://clips.example"}, withPath.TrustedOrigins)
}

func TestCookieSecureIsFalseForLocalHTTP(t *testing.T) {
	t.Parallel()

	assert.False(t, cookieSecure(&config.Config{ListenAddr: ":8080"}))
	assert.True(t, cookieSecure(&config.Config{PublicBaseURL: "https://clips.example"}))
}

func TestCSRFTrustedOriginsIsEmptyWithoutAPublicBaseURL(t *testing.T) {
	t.Parallel()

	assert.Nil(t, csrfTrustedOrigins(&config.Config{ListenAddr: "127.0.0.1:8080"}))
}

func TestCSRFTrustedOriginsRejectsAnUnparseableURL(t *testing.T) {
	t.Parallel()

	assert.Nil(t, csrfTrustedOrigins(&config.Config{PublicBaseURL: "https://clips.example/%zz"}))
}

func TestCSRFTrustedOriginsRejectsAURLWithoutAScheme(t *testing.T) {
	t.Parallel()

	assert.Nil(t, csrfTrustedOrigins(&config.Config{PublicBaseURL: "clips.example/outtake"}))
}

func TestCSRFTrustedOriginsRejectsAURLWithoutAHost(t *testing.T) {
	t.Parallel()

	assert.Nil(t, csrfTrustedOrigins(&config.Config{PublicBaseURL: "https://"}))
}

func TestCookieSecureAcceptsAnUppercaseHTTPSScheme(t *testing.T) {
	t.Parallel()

	assert.True(t, cookieSecure(&config.Config{PublicBaseURL: "HTTPS://https://clips.example"}))
	assert.False(t, cookieSecure(&config.Config{PublicBaseURL: "HTTP://https://clips.example"}))
}

func TestCookieSecureIsFalseWithoutAListenAddress(t *testing.T) {
	t.Parallel()

	assert.False(t, cookieSecure(&config.Config{}))
}

func TestSessionConfigSetsTheIdleAndAbsoluteTimeouts(t *testing.T) {
	t.Parallel()

	cfg := sessionConfig(&config.Config{ListenAddr: ":8080"}, nil)

	assert.Equal(t, 30*time.Minute, cfg.IdleTimeout)
	assert.Equal(t, 24*time.Hour, cfg.AbsoluteTimeout)
	assert.Equal(t, "Lax", cfg.CookieSameSite)
	assert.True(t, cfg.CookieHTTPOnly)
	assert.False(t, cfg.CookieSecure)
	assert.False(t, cfg.CookieSessionOnly)
	assert.Empty(t, cfg.CookieDomain)
	assert.Empty(t, cfg.CookiePath)
}

func TestSessionCookieSecureFollowsPublicURL(t *testing.T) {
	t.Parallel()

	local := sessionConfig(&config.Config{ListenAddr: "127.0.0.1:8080"}, nil)
	assert.False(t, local.CookieSecure)

	published := sessionConfig(&config.Config{PublicBaseURL: "https://clips.example"}, nil)
	assert.True(t, published.CookieSecure)
}

func TestSessionConfigLeavesEveryOptionalHookUnset(t *testing.T) {
	t.Parallel()

	cfg := sessionConfig(&config.Config{}, nil)

	assert.Nil(t, cfg.Storage)
	assert.Nil(t, cfg.Store)
	assert.Nil(t, cfg.Next)
	assert.Nil(t, cfg.ErrorHandler)
	assert.Nil(t, cfg.KeyGenerator)
}

func TestSessionMiddlewareNamesTheCookieSessionID(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(session.New(sessionConfig(&config.Config{}, nil)))
	app.Get("/session/probe", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/session/probe",
		http.NoBody,
	)

	resp, err := app.Test(req)
	require.NoError(t, err)

	require.NoError(t, resp.Body.Close())

	names := make([]string, 0, len(resp.Cookies()))

	for _, cookie := range resp.Cookies() {
		names = append(names, cookie.Name)
	}

	assert.Contains(t, names, "session_id")
}

func TestHelmetConfigSetsEverySecurityHeader(t *testing.T) {
	t.Parallel()

	cfg := helmetConfig()

	assert.Equal(t, "0", cfg.XSSProtection)
	assert.Equal(t, "nosniff", cfg.ContentTypeNosniff)
	assert.Equal(t, "DENY", cfg.XFrameOptions)
	assert.Equal(t, contentSecurityPolicy, cfg.ContentSecurityPolicy)
	assert.Contains(t, cfg.ContentSecurityPolicy, "; object-src 'none';")
	assert.NotContains(t, cfg.ContentSecurityPolicy, "'object-src")
	assert.Equal(t, "no-referrer", cfg.ReferrerPolicy)
	assert.Empty(t, cfg.PermissionPolicy)
	assert.Nil(t, cfg.Next)
}

func TestHelmetConfigIsolatesTheDocument(t *testing.T) {
	t.Parallel()

	cfg := helmetConfig()

	assert.Equal(t, "require-corp", cfg.CrossOriginEmbedderPolicy)
	assert.Equal(t, "same-origin", cfg.CrossOriginOpenerPolicy)
	assert.Equal(t, "same-origin", cfg.CrossOriginResourcePolicy)
	assert.Equal(t, "?1", cfg.OriginAgentCluster)
}

func TestHelmetConfigLeavesTransportSecurityOff(t *testing.T) {
	t.Parallel()

	cfg := helmetConfig()

	assert.Equal(t, 0, cfg.HSTSMaxAge)
	assert.False(t, cfg.HSTSExcludeSubdomains)
	assert.False(t, cfg.HSTSPreloadEnabled)
	assert.False(t, cfg.CSPReportOnly)
}

func TestHelmetConfigGuardsAgainstSniffingAndDownloading(t *testing.T) {
	t.Parallel()

	cfg := helmetConfig()

	assert.Equal(t, "off", cfg.XDNSPrefetchControl)
	assert.Equal(t, "noopen", cfg.XDownloadOptions)
	assert.Equal(t, "none", cfg.XPermittedCrossDomain)
}

func TestHelmetMiddlewareWritesTheConfiguredHeaders(t *testing.T) {
	t.Parallel()

	headers := helmetHeaders(t, helmetConfig())

	assert.Equal(t, "require-corp", headers.Get("Cross-Origin-Embedder-Policy"))
	assert.Equal(t, "same-origin", headers.Get("Cross-Origin-Opener-Policy"))
	assert.Equal(t, "same-origin", headers.Get("Cross-Origin-Resource-Policy"))
	assert.Equal(t, "?1", headers.Get("Origin-Agent-Cluster"))
	assert.Equal(t, "DENY", headers.Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", headers.Get("Referrer-Policy"))
	assert.Empty(t, headers.Get("Strict-Transport-Security"),
		"a zero max age emits no transport security header at all")
}

func TestCSRFErrorReportsForbidden(t *testing.T) {
	t.Parallel()

	err := csrfError(nil, assert.AnError)

	var fiberErr *fiber.Error

	require.ErrorAs(t, err, &fiberErr)
	assert.Equal(t, fiber.StatusForbidden, fiberErr.Code)
	assert.Equal(t, "invalid csrf token", fiberErr.Message)
}

func TestCSRFMiddlewareAnswersAMissingTokenThroughTheErrorHandler(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(csrf.New(csrfConfig(&config.Config{ListenAddr: "127.0.0.1:8080"}, nil)))
	app.Get("/csrf/probe", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	app.Post("/csrf/probe", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/csrf/probe", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)

	require.NoError(t, resp.Body.Close())

	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode,
		"a request carrying no CSRF cookie is refused before the handler runs")
}

func TestCSRFMiddlewareLetsAGetRequestThrough(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(csrf.New(csrfConfig(&config.Config{ListenAddr: "127.0.0.1:8080"}, nil)))
	app.Get("/csrf/probe", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/csrf/probe", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)

	require.NoError(t, resp.Body.Close())

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestCSRFConfigWiresTheHeaderAndFormExtractors(t *testing.T) {
	t.Parallel()

	cfg := csrfConfig(&config.Config{ListenAddr: "127.0.0.1:8080"}, nil)

	assert.Equal(t, "csrf_", cfg.CookieName)
	assert.Equal(t, "Lax", cfg.CookieSameSite)
	assert.Equal(t, 30*time.Minute, cfg.IdleTimeout)
	assert.True(t, cfg.CookieHTTPOnly)
	assert.False(t, cfg.CookieSessionOnly)
	assert.False(t, cfg.SingleUseToken)
	assert.False(t, cfg.DisableValueRedaction)
	assert.NotNil(t, cfg.KeyGenerator)
	assert.NotNil(t, cfg.ErrorHandler)
	assert.NotNil(t, cfg.Extractor)
	assert.Empty(t, cfg.CookieDomain)
	assert.Empty(t, cfg.CookiePath)
}

func TestCSRFConfigLeavesEveryOptionalHookUnset(t *testing.T) {
	t.Parallel()

	cfg := csrfConfig(&config.Config{ListenAddr: "127.0.0.1:8080"}, nil)

	assert.Nil(t, cfg.Storage)
	assert.Nil(t, cfg.Next)
	assert.Nil(t, cfg.Session)
}

func TestCSRFConfigExtractsTheHeaderBeforeTheFormField(t *testing.T) {
	t.Parallel()

	cfg := csrfConfig(&config.Config{ListenAddr: "127.0.0.1:8080"}, nil)

	assert.Equal(t, csrf.HeaderName, cfg.Extractor.Key)
	assert.Len(t, cfg.Extractor.Chain, 2)
	assert.Equal(t, "header-token",
		extractedToken(t, cfg.Extractor.Extract, csrfSource("header")))
	assert.Equal(t, "form-token",
		extractedToken(t, cfg.Extractor.Extract, csrfSource("form")))
}

// extractedToken runs the configured CSRF extractor against a request that
// carries the token in one named place.
//
// Parameters:
//   - t: The test that ran the extractor.
//   - extract: The extractor under test.
//   - source: Where the request carries its token.
//
// Returns:
//   - token: The token the extractor returned.
func extractedToken(
	t *testing.T,
	extract func(fiber.Ctx) (string, error),
	source csrfSource,
) string {
	t.Helper()

	app := fiber.New()
	app.Post("/csrf/extract", func(ctx fiber.Ctx) error {
		token, err := extract(ctx)
		require.NoError(t, err)

		return ctx.SendString(token)
	})

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/csrf/extract",
		strings.NewReader(identity.CSRFFormField+"=form-token"),
	)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	if source == csrfSource("header") {
		req.Header.Set(csrf.HeaderName, "header-token")
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	return readBody(t, resp)
}

// readBody drains a response body and closes it.
//
// Parameters:
//   - t: The test that read the body.
//   - resp: The response whose body is drained.
//
// Returns:
//   - body: The response body as a string.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return string(body)
}

func TestStaticConfigServesTheEmbeddedFileSystem(t *testing.T) {
	t.Parallel()

	cfg := staticConfig()

	assert.Equal(t, Assets, cfg.FS)
	assert.Equal(t, []string{"index.html"}, cfg.IndexNames)
	assert.Equal(t, time.Duration(0), cfg.CacheDuration)
	assert.Equal(t, 0, cfg.MaxAge)
}

func TestStaticConfigLeavesEveryOptionalBehaviourOff(t *testing.T) {
	t.Parallel()

	cfg := staticConfig()

	assert.Nil(t, cfg.Next)
	assert.Nil(t, cfg.ModifyResponse)
	assert.Nil(t, cfg.NotFoundHandler)
	assert.False(t, cfg.Compress)
	assert.False(t, cfg.ByteRange)
	assert.False(t, cfg.Browse)
	assert.False(t, cfg.Download)
}

// helmetHeaders runs one request through the helmet middleware and returns the
// response headers it produced.
//
// Parameters:
//   - t: The test that issued the request.
//   - cfg: Helmet configuration to install.
//
// Returns:
//   - headers: The response headers.
func helmetHeaders(t *testing.T, cfg helmet.Config) http.Header {
	t.Helper()

	app := fiber.New()
	app.Use(helmet.New(cfg))
	app.Get("/headers/probe", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/headers/probe",
		http.NoBody,
	)

	probed, err := app.Test(req)
	require.NoError(t, err)

	require.NoError(t, probed.Body.Close())

	return probed.Header
}

func TestSessionAndCSRFConfigsUseTheGivenStores(t *testing.T) {
	t.Parallel()

	storage := database.NewSessionStore(testRouterDatabase(t))
	cfg := &config.Config{ListenAddr: "127.0.0.1:8080"}

	sessions := session.NewStore(sessionConfig(cfg, storage))

	assert.Same(
		t,
		storage,
		sessionConfig(cfg, storage).Storage,
		"sessions persist in the given storage",
	)
	assert.Same(t, sessions, csrfConfig(cfg, sessions).Session, "CSRF tokens live in the session")
}
