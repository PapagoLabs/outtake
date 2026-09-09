// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"net/url"
	"strings"
	"time"

	fiber "github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/csrf"

	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/web"
)

// CSRFConfig returns CSRF middleware that accepts header or form tokens.
//
// Parameters:
//   - cfg: Application configuration.
//
// Returns:
//   - cfg: The CSRF middleware that accepts header or form tokens.
func CSRFConfig(cfg *config.Config) csrf.Config {
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
		TrustedOrigins: CSRFTrustedOrigins(cfg),
		Extractor: extractors.Chain(
			extractors.FromHeader(csrf.HeaderName),
			extractors.FromForm(web.CSRFFormField),
		),
		IdleTimeout:           sessionIdleMinutes * time.Minute,
		DisableValueRedaction: false,
		CookieSecure:          CookieSecure(cfg),
		CookieHTTPOnly:        true,
		CookieSessionOnly:     false,
		SingleUseToken:        false,
	}
}

// CSRFTrustedOrigins returns Fiber CSRF TrustedOrigins from PublicBaseURL.
//
// Parameters:
//   - cfg: Application configuration.
//
// Returns:
//   - items: The Fiber CSRF TrustedOrigins from PublicBaseURL.
func CSRFTrustedOrigins(cfg *config.Config) []string {
	if cfg.PublicBaseURL == "" {
		return nil
	}

	parsed, err := url.Parse(cfg.PublicURL())
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}

	return []string{parsed.Scheme + "://" + parsed.Host}
}

// CookieSecure reports whether cookies should set the Secure attribute.
//
// Parameters:
//   - cfg: Application configuration.
//
// Returns:
//   - ok: True when cookies should set the Secure attribute.
func CookieSecure(cfg *config.Config) bool {
	return strings.HasPrefix(strings.ToLower(cfg.PublicURL()), "https://")
}

// csrfError renders a CSRF failure response.
//
// Returns:
//   - err: The error, if any.
func csrfError(_ fiber.Ctx, _ error) error {
	return fiber.NewError(fiber.StatusForbidden, "invalid csrf token")
}
