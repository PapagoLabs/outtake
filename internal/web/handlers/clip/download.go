// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"fmt"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

const (
	// msgNotReady explains why a clip cannot be downloaded yet.
	msgNotReady = "clip is not ready for download"

	// msgFileMissing explains that the output is not on disk.
	msgFileMissing = "output file not found on disk"
)

// Download handles the download clip request.
//
// Parameters:
//   - ctx: Request context carrying the clip id route parameter.
//
// Returns:
//   - err: Response error, or nil on success.
func (handler *Handler) Download(ctx fiber.Ctx) error {
	id := ctx.Params(routes.ParamID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return respond.WriteNotFound(ctx)
	}

	if job.Status != clipdom.StatusCompleted {
		return respond.WriteError(
			ctx,
			fiber.StatusConflict,
			api.NotReady,
			msgNotReady,
		)
	}

	if handler.clipStorage.Ensure(ctx.Context(), job.OutputPath) != nil {
		return respond.WriteError(
			ctx,
			fiber.StatusNotFound,
			api.FileMissing,
			msgFileMissing,
		)
	}

	ctx.Attachment(job.Filename())

	err := ctx.SendFile(job.OutputPath)
	if err != nil {
		return fmt.Errorf("send file: %w", err)
	}

	return nil
}
