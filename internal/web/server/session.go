// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"time"

	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/session"
)

const (
	sessionIdleMinutes   = 30
	sessionAbsoluteHours = 24
)

// SessionConfig returns the Fiber session middleware configuration.
func SessionConfig() session.Config {
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
		IdleTimeout:       sessionIdleMinutes * time.Minute,
		AbsoluteTimeout:   sessionAbsoluteHours * time.Hour,
		CookieSecure:      false,
		CookieHTTPOnly:    true,
		CookieSessionOnly: false,
	}
}
