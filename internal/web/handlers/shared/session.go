// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/binding"
	"github.com/PapagoLabs/outtake/internal/web/api"
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
	return WriteJSON(ctx, fiber.StatusOK, SessionResponses(handler.bind.Sessions()))
}

// SessionResponses maps Plex sessions onto API payloads.
func SessionResponses(sessions []plex.Session) []api.SessionResponse {
	responses := make([]api.SessionResponse, 0, len(sessions))

	for index := range sessions {
		sess := &sessions[index]

		responses = append(responses, api.SessionResponse{
			ID:         sess.ID,
			MediaID:    sess.MediaItem.ID,
			Title:      sess.MediaItem.DisplayTitle(),
			Duration:   sess.Duration,
			ViewOffset: sess.ViewOffset,
		})
	}

	return responses
}
