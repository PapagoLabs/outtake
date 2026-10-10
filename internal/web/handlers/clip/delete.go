// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// msgDeleteFailed is shown when a clip or its files cannot be removed.
const msgDeleteFailed = "Couldn't delete the clip. Check the Outtake log."

// Delete handles the delete clip request.
//
// Parameters:
//   - ctx: Request context carrying the clip id route parameter.
//
// Returns:
//   - err: Response error, or nil on success.
func (handler *Handler) Delete(ctx fiber.Ctx) error {
	id := ctx.Params(routes.ParamID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return respond.WriteNotFound(ctx)
	}

	// The row goes before the file. Removing the file first meant a row that
	// could not be deleted came back pointing at a file that was already gone.
	handler.clipQueue.Delete(id)

	err := handler.db.DeleteClip(ctx.Context(), id)
	if err != nil {
		handler.reinstateClip(ctx, job)

		return respond.WriteFailure(
			ctx,
			fiber.StatusInternalServerError,
			api.DeleteFailed,
			respond.FailWith(ctx, msgDeleteFailed, err),
		)
	}

	// A video clip's SDR version goes with it.
	for _, path := range []string{job.OutputPath, job.SDRPath()} {
		if path == "" {
			continue
		}

		fileErr := handler.clipStorage.DeleteFile(path)
		if fileErr != nil {
			// The row is already gone, so put the clip back as it was. Marking
			// it canceled would hide a finished file the delete did not remove.
			handler.restoreClip(ctx, job)

			return respond.WriteFailure(
				ctx,
				fiber.StatusInternalServerError,
				api.DeleteFailed,
				respond.FailWith(ctx, msgDeleteFailed, fileErr),
			)
		}
	}

	return respond.SendStatusCode(ctx, fiber.StatusOK)
}

// reinstateClip puts a clip back after the row delete failed, canceled so a
// later start does not resume a render the user asked to stop.
//
// Parameters:
//   - ctx: Request context.
//   - job: The clip Delete took out of the queue.
func (handler *Handler) reinstateClip(ctx fiber.Ctx, job *clipdom.Job) {
	handler.clipQueue.Reinstate(job)
	handler.persistRestoredClip(ctx, job)
}

// restoreClip puts a clip back after its file could not be removed, keeping
// the status it had so a finished clip stays downloadable.
//
// Parameters:
//   - ctx: Request context.
//   - job: The clip whose row was removed and has to be written again.
func (handler *Handler) restoreClip(ctx fiber.Ctx, job *clipdom.Job) {
	handler.clipQueue.Restore(job)
	handler.persistRestoredClip(ctx, job)
}

// persistRestoredClip writes a clip that a failed delete put back.
//
// Parameters:
//   - ctx: Request context.
//   - job: The clip to store again.
func (handler *Handler) persistRestoredClip(ctx fiber.Ctx, job *clipdom.Job) {
	saveErr := handler.db.SaveClip(ctx.Context(), job)
	if saveErr != nil {
		logging.Logger.Warn().
			Str("job_id", job.ID).
			Err(saveErr).
			Msg("failed to persist reinstated clip")
	}
}
