// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// GetStatus handles the get clip status request.
//
// Parameters:
//   - ctx: Request context carrying the clip id route parameter.
//
// Returns:
//   - err: Response error, or nil on success.
func (handler *Handler) GetStatus(ctx fiber.Ctx) error {
	id := ctx.Params(routes.ParamID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return respond.WriteNotFound(ctx)
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, catalog.ClipResponse(job))
}
