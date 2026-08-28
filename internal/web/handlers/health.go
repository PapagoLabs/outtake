// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	fiber "github.com/gofiber/fiber/v3"
)

// HealthHandler handles health check requests.
type HealthHandler struct{}

// NewHealthHandler creates a new health handler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health handles the health check request.
func (*HealthHandler) Health(c fiber.Ctx) error {
	return writeJSON(c, fiber.StatusOK, fiber.Map{
		"status":  "ok",
		"service": "outtake",
	})
}
