// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"fmt"
	"strings"

	fiber "github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
)

const (
	// e2eEnv is the environment name that skips authentication.
	e2eEnv = "e2e"
)

// AuthGuard provides authentication middleware that checks if the user has.
//
// Parameters:
//   - env: App environment (development vs production).
//
// Returns:
//   - handler: The authentication middleware that checks if the user has.
func AuthGuard(env string) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		if env == e2eEnv {
			return ctx.Next()
		}

		if tokenFromSession(session.FromContext(ctx)) == "" {
			return unauthenticated(ctx)
		}

		return ctx.Next()
	}
}

// RestoreToken loads a persisted Plex token into the session when missing.
//
// Parameters:
//   - db: Database handle.
//
// Returns:
//   - handler: A persisted Plex token into the session when missing.
func RestoreToken(db *database.DB) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		sess := session.FromContext(ctx)
		if sess == nil || tokenFromSession(sess) != "" {
			return ctx.Next()
		}

		stored, err := db.LatestToken(ctx.Context())
		if err == nil && stored != "" {
			sess.Set(SessionKeyToken, stored)
		}

		return ctx.Next()
	}
}

// tokenFromSession reads the Plex token from the Fiber session.
//
// Parameters:
//   - sess: Fiber session store entry.
//
// Returns:
//   - value: The Plex token from the Fiber session.
func tokenFromSession(sess *session.Middleware) string {
	if sess == nil {
		return ""
	}

	token, ok := sess.Get(SessionKeyToken).(string)
	if !ok {
		return ""
	}

	return token
}

// unauthenticated rejects an anonymous request.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: Wrapped failure such as "write unauthorized" or "redirect login".
func unauthenticated(ctx fiber.Ctx) error {
	if strings.HasPrefix(ctx.Path(), "/api/") {
		err := ctx.Status(fiber.StatusUnauthorized).JSON(respond.ErrorResponse{
			Error:   "unauthorized",
			Message: "authentication required",
		})
		if err != nil {
			return fmt.Errorf("write unauthorized: %w", err)
		}

		return nil
	}

	err := ctx.Redirect().To("/login")
	if err != nil {
		return fmt.Errorf("redirect login: %w", err)
	}

	return nil
}
