// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// Cancel stops a pending or processing clip.
//
// Parameters:
//   - ctx: Request context carrying the clip id route parameter.
//
// Returns:
//   - err: Response or redirect error, or nil on success.
func (handler *Handler) Cancel(ctx fiber.Ctx) error {
	id := ctx.Params(routes.ParamID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return respond.WriteNotFound(ctx)
	}

	if !handler.clipQueue.Cancel(id) {
		return respond.WriteError(
			ctx,
			fiber.StatusConflict,
			api.NotCancellable,
			"clip is not running",
		)
	}

	// The queue marks the job canceled and persists that itself. Reading it
	// again lets the response carry the status after the cancel.
	if canceled := handler.lookupJob(ctx.Context(), id); canceled != nil {
		job = canceled
	}

	if respond.IsHTMXRequest(ctx) {
		ctx.Set(routes.HeaderHXRefresh, "true")

		return respond.SendStatusCode(ctx, fiber.StatusOK)
	}

	if respond.IsFormRequest(ctx) {
		return respond.RedirectTo(ctx, ReturnPath(job.MediaID))
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, catalog.ClipResponse(job))
}
