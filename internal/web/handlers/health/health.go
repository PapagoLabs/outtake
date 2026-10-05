// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package health

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/respond"
)

// Handler handles health check requests.
type Handler struct{}

// New creates a new health handler.
//
// Returns:
//   - handler: A health handler with no collaborators.
func New() *Handler {
	return &Handler{}
}

// Health handles the health check request.
//
// Parameters:
//   - c: Request context.
//
// Returns:
//   - err: Response error, or nil on success.
func (*Handler) Health(c fiber.Ctx) error {
	return respond.WriteJSON(c, fiber.StatusOK, fiber.Map{
		"status":  "ok",
		"service": "outtake",
	})
}
