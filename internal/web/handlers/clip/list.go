// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/web/respond"
)

// List handles the list clips request.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Response error, or nil on success.
func (handler *Handler) List(ctx fiber.Ctx) error {
	return respond.WriteJSON(
		ctx,
		fiber.StatusOK,
		fiber.Map{"clips": catalog.ClipResponses(handler.listJobs(ctx.Context()))},
	)
}
