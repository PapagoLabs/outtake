// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package health

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
)

// HealthHandler handles health check requests.
type HealthHandler struct{}

// NewHealthHandler creates a new health handler.
//
// Returns:
//   - healthHandler: A new health handler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health handles the health check request.
//
// Parameters:
//   - c: HTTP request context.
//
// Returns:
//   - err: Non-nil when the health JSON response cannot be written.
func (*HealthHandler) Health(c fiber.Ctx) error {
	return respond.WriteJSON(c, fiber.StatusOK, fiber.Map{
		"status":  "ok",
		"service": "outtake",
	})
}
