// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3/middleware/session"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
)

const (
	// e2eEnv is the environment name that skips authentication.
	e2eEnv = "e2e"
)

// AuthGuard rejects a request that has no Plex token in its session.
//
// Parameters:
//   - env: Configured environment. The e2e environment skips the check.
//
// Returns:
//   - handler: Middleware that redirects or answers anonymous requests.
func AuthGuard(env string) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		if env == e2eEnv {
			return ctx.Next()
		}

		if identity.Token(session.FromContext(ctx)) == "" {
			return unauthenticated(ctx)
		}

		return ctx.Next()
	}
}

// RestoreToken loads a persisted Plex token into the session when missing.
//
// Parameters:
//   - store: Persisted token store.
//
// Returns:
//   - handler: Middleware that seeds the session and continues the chain.
func RestoreToken(store identity.TokenStore) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		sess := session.FromContext(ctx)
		if sess == nil || identity.Token(sess) != "" {
			return ctx.Next()
		}

		identity.SetToken(sess, identity.Restore(ctx.Context(), store))

		return ctx.Next()
	}
}

// unauthenticated rejects an anonymous request.
//
// Parameters:
//   - ctx: Request context for the anonymous request.
//
// Returns:
//   - err: The write or redirect failure, or nil once the response is sent.
func unauthenticated(ctx fiber.Ctx) error {
	if strings.HasPrefix(ctx.Path(), "/api/") {
		err := ctx.Status(fiber.StatusUnauthorized).JSON(api.ErrorResponse{
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
