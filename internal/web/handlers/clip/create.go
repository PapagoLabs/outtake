// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"time"
	"uuid"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	clipprofile "github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/web/respond"
)

// Create handles the create clip request.
//
// Parameters:
//   - ctx: Request context carrying the create fields.
//
// Returns:
//   - err: Response or redirect error, or nil on success.
func (handler *Handler) Create(ctx fiber.Ctx) error {
	req, err := ParseRequest(ctx)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, api.InvalidRequest, err.Error())
	}

	jobType, err := clipdom.ResolveType(req.ClipType, "")
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, api.InvalidClipType, err.Error())
	}

	inputPath, failCode, err := handler.resolveNewClip(ctx, &req, jobType)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, failCode, err.Error())
	}

	job := buildJob(
		&req,
		jobType,
		inputPath,
		handler.keepHDR(ctx.Context(), req.Quality),
	)

	job.OutputPath = handler.clipPaths.OutputPath(job.ID, job.Type)

	job.ApplyDefaults()

	err = handler.db.SaveClip(ctx.Context(), job)
	if err != nil {
		return respond.WriteError(
			ctx,
			fiber.StatusInternalServerError,
			api.PersistFailed,
			err.Error(),
		)
	}

	err = handler.clipQueue.Submit(job)
	if err != nil {
		//nolint:wrapcheck // the error becomes a response body, not a returned chain.
		return writeJobSubmitError(ctx, err)
	}

	if respond.IsFormRequest(ctx) {
		return respond.RedirectTo(ctx, ReturnPath(req.MediaID))
	}

	return respond.WriteJSON(ctx, fiber.StatusCreated, catalog.ClipResponse(job))
}

// buildJob constructs a pending queue job from a clip request.
//
// Parameters:
//   - req: The resolved request.
//   - jobType: Normalized job type.
//   - inputPath: Resolved source media path.
//   - preserveHDR: Resolved keep-HDR setting.
//
// Returns:
//   - job: The pending clip record handed to persistence and the queue.
func buildJob(
	req *api.ClipRequest,
	jobType clipdom.Type,
	inputPath string,
	preserveHDR bool,
) *clipdom.Job {
	start, duration := requestWindow(*req)
	now := time.Now()

	return &clipdom.Job{
		ID:            uuid.New().String(),
		Type:          jobType,
		Name:          clipdom.DisplayName(req.Name, req.MediaTitle),
		MediaID:       req.MediaID,
		MediaTitle:    req.MediaTitle,
		MediaType:     req.MediaType,
		StartTime:     start,
		Duration:      duration,
		Quality:       req.Quality,
		Width:         req.Width,
		FPS:           req.FPS,
		AudioIndex:    req.AudioIndex,
		CropBlackBars: req.CropBlackBars,
		PreserveHDR:   preserveHDR,
		CreatedAt:     now,
		UpdatedAt:     now,
		InputPath:     inputPath,
		OutputPath:    "",
		Status:        clipdom.StatusPending,
		Progress:      0,
		Error:         "",
		Stage:         clipdom.StageClip,
		OutputFormat:  clipdom.Format{},
		SDRFormat:     clipdom.Format{},
	}
}

// keepHDR reports whether a clip rendered with a profile keeps an HDR source's
// HDR, which is the profile's setting.
//
// Parameters:
//   - ctx: Request scope for the profile lookup.
//   - quality: The profile id.
//
// Returns:
//   - keep: True when the profile keeps HDR.
func (handler *Handler) keepHDR(ctx context.Context, quality string) bool {
	return clipprofile.Preset(ctx, handler.db, quality).PreserveHDR
}
