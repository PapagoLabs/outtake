// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"github.com/gofiber/fiber/v3/middleware/csrf"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web"
)

// BindCSRFToken copies the Fiber CSRF token onto the request context for templates.
//
// Returns:
//   - Middleware that stores the token and continues the chain.
func BindCSRFToken() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		token := csrf.TokenFromContext(ctx)
		ctx.SetContext(web.ContextWithCSRFToken(ctx.Context(), token))

		return ctx.Next()
	}
}
