// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"fmt"
	"time"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/logging"
)

// RequestLogger provides request logging middleware.
func RequestLogger() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		start := time.Now()
		err := ctx.Next()
		duration := time.Since(start)

		logging.Logger.Info().
			Str("method", ctx.Method()).
			Str("path", ctx.Path()).
			Int("status", ctx.Response().StatusCode()).
			Dur("duration", duration).
			Msg("request")

		if err != nil {
			return fmt.Errorf("request logger: %w", err)
		}

		return nil
	}
}
