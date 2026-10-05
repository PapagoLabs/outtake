// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// contentSecurityPolicy is the helmet CSP for vendored HTMX and same-origin media.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self'; media-src 'self'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// sessionIdleTimeout is the session idle timeout.
const sessionIdleTimeout = 30 * time.Minute

// sessionAbsoluteTimeout is the session absolute timeout.
const sessionAbsoluteTimeout = 24 * time.Hour

// sessionConfig returns the Fiber session middleware configuration.
//
// Parameters:
//   - cfg: App config. PublicURL controls CookieSecure, matching the CSRF cookie.
//
// Returns:
//   - config: Session middleware configuration.
func sessionConfig(cfg *config.Config) session.Config {
	return session.Config{
		Storage:           nil,
		Store:             nil,
		Next:              nil,
		ErrorHandler:      nil,
		KeyGenerator:      nil,
		CookieDomain:      "",
		CookiePath:        "",
		CookieSameSite:    "Lax",
		Extractor:         extractors.FromCookie("session_id"),
		IdleTimeout:       sessionIdleTimeout,
		AbsoluteTimeout:   sessionAbsoluteTimeout,
		CookieSecure:      cookieSecure(cfg),
		CookieHTTPOnly:    true,
		CookieSessionOnly: false,
	}
}

// helmetConfig returns security headers including a same-origin CSP.
//
// Returns:
//   - helmet: Middleware config for the security headers.
func helmetConfig() helmet.Config {
	return helmet.Config{
		Next:                      nil,
		XSSProtection:             "0",
		ContentTypeNosniff:        "nosniff",
		XFrameOptions:             "DENY",
		ContentSecurityPolicy:     contentSecurityPolicy,
		ReferrerPolicy:            "no-referrer",
		PermissionPolicy:          "",
		CrossOriginEmbedderPolicy: "require-corp",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
		OriginAgentCluster:        "?1",
		XDNSPrefetchControl:       "off",
		XDownloadOptions:          "noopen",
		XPermittedCrossDomain:     "none",
		HSTSMaxAge:                0,
		HSTSExcludeSubdomains:     false,
		CSPReportOnly:             false,
		HSTSPreloadEnabled:        false,
	}
}

// csrfConfig returns CSRF middleware that accepts header or form tokens.
//
// Parameters:
//   - cfg: App config. PublicURL controls CookieSecure and TrustedOrigins.
//
// Returns:
//   - config: CSRF middleware config.
func csrfConfig(cfg *config.Config) csrf.Config {
	return csrf.Config{
		Storage:        nil,
		Next:           nil,
		Session:        nil,
		KeyGenerator:   csrf.ConfigDefault.KeyGenerator,
		ErrorHandler:   csrfError,
		CookieName:     "csrf_",
		CookieDomain:   "",
		CookiePath:     "",
		CookieSameSite: "Lax",
		TrustedOrigins: csrfTrustedOrigins(cfg),
		Extractor: extractors.Chain(
			extractors.FromHeader(csrf.HeaderName),
			extractors.FromForm(identity.CSRFFormField),
		),
		IdleTimeout:           sessionIdleTimeout,
		DisableValueRedaction: false,
		CookieSecure:          cookieSecure(cfg),
		CookieHTTPOnly:        true,
		CookieSessionOnly:     false,
		SingleUseToken:        false,
	}
}

// csrfTrustedOrigins returns Fiber CSRF TrustedOrigins from PublicBaseURL.
//
// Parameters:
//   - cfg: App config. PublicBaseURL is the reverse-proxy origin when set.
//
// Returns:
//   - origins: Scheme and host from PublicURL, or nil when PublicBaseURL is unset.
func csrfTrustedOrigins(cfg *config.Config) []string {
	if cfg.PublicBaseURL == "" {
		return nil
	}

	parsed, err := url.Parse(cfg.PublicURL())
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}

	return []string{parsed.Scheme + "://" + parsed.Host}
}

// cookieSecure reports whether cookies should set the Secure attribute.
//
// Parameters:
//   - cfg: App config. PublicURL is https for remote HTTPS deployments.
//
// Returns:
//   - True when PublicURL uses https; false for local HTTP.
func cookieSecure(cfg *config.Config) bool {
	return strings.HasPrefix(strings.ToLower(cfg.PublicURL()), "https://")
}

// csrfError turns a CSRF failure into a Fiber error for PageError.
//
// Parameters:
//   - _ctx: Request context. Unused.
//   - _err: CSRF middleware error. Unused.
//
// Returns:
//   - Forbidden Fiber error.
func csrfError(_ fiber.Ctx, _ error) error {
	return fiber.NewError(fiber.StatusForbidden, "invalid csrf token")
}

// staticConfig returns the static asset middleware configuration.
//
// Returns:
//   - config: Static asset middleware configuration.
func staticConfig() static.Config {
	return static.Config{
		FS:              Assets,
		Next:            nil,
		ModifyResponse:  nil,
		NotFoundHandler: nil,
		IndexNames:      []string{"index.html"},
		CacheDuration:   0,
		MaxAge:          0,
		Compress:        false,
		ByteRange:       false,
		Browse:          false,
		Download:        false,
	}
}
