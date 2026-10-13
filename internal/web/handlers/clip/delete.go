// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"fmt"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
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
	job := handler.lookupJob(ctx.Context(), middleware.ClipScope(ctx), id)
	if job == nil {
		return respond.WriteNotFound(ctx)
	}

	err := handler.deleteJob(ctx, job)
	if err != nil {
		return respond.WriteFailure(
			ctx,
			fiber.StatusInternalServerError,
			api.DeleteFailed,
			respond.FailWith(ctx, msgDeleteFailed, err),
		)
	}

	return respond.SendStatusCode(ctx, fiber.StatusOK)
}

// deleteJob removes a clip, its row, and its files.
//
// The row goes before the files, so a row that cannot be deleted never comes
// back pointing at a file that is already gone. A clip whose row or files
// cannot be removed is put back.
//
// Parameters:
//   - ctx: Request context.
//   - job: The clip, which the caller may change.
//
// Returns:
//   - err: Non-nil when the row or a file cannot be removed.
func (handler *Handler) deleteJob(ctx fiber.Ctx, job *clipdom.Job) error {
	handler.clipQueue.Delete(job.ID)

	err := handler.db.DeleteClip(ctx.Context(), job.ID)
	if err != nil {
		handler.reinstateClip(ctx, job)

		return fmt.Errorf("delete clip row: %w", err)
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

			return fmt.Errorf("delete clip file: %w", fileErr)
		}
	}

	return nil
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
