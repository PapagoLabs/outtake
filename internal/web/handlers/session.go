// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/binding"
)

// SessionHandler handles session-related requests.
type SessionHandler struct {
	bind *binding.Binding
}

// NewSessionHandler creates a new session handler.
func NewSessionHandler(bind *binding.Binding) *SessionHandler {
	return &SessionHandler{bind: bind}
}

// List handles the list sessions request.
func (handler *SessionHandler) List(ctx fiber.Ctx) error {
	return writeJSON(ctx, fiber.StatusOK, sessionResponses(handler.bind.Sessions()))
}
