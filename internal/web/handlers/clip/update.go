// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"fmt"
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	clipcard "github.com/PapagoLabs/outtake/internal/web/components/clip"
	"github.com/PapagoLabs/outtake/internal/web/components/flash"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// clipRejection is a rejected clip edit, held rather than written.
type clipRejection struct {
	status  int
	code    api.ErrorCode
	message string
}

// Update saves clip metadata and optionally regenerates the file.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) Update(ctx fiber.Ctx) error {
	job, failure := handler.stageClipUpdate(ctx)
	if failure != nil {
		//nolint:wrapcheck // The helper writes the response itself.
		return respondWithUpdateFailure(ctx, failure)
	}

	//nolint:wrapcheck // The helper writes the response itself.
	return handler.respondWithClipCard(ctx, job)
}

// stageClipUpdate runs every check, applies the edit, and saves it.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - job: The saved clip, or nil when the edit was rejected.
//   - failure: Why the edit was rejected, or nil when it was saved.
func (handler *Handler) stageClipUpdate(ctx fiber.Ctx) (*clipdom.Job, *clipRejection) {
	job := handler.lookupJob(ctx.Context(), ctx.Params(routes.ParamID))
	if job == nil {
		return nil, newRejection(fiber.StatusNotFound, api.NotFound, respond.NotFoundMessage)
	}

	req, err := ParseRequest(ctx)
	if err != nil {
		return nil, newRejection(fiber.StatusBadRequest, api.InvalidRequest, err.Error())
	}

	err = handler.applyRequestQuality(ctx.Context(), &req)
	if err != nil {
		return nil, newRejection(fiber.StatusBadRequest, api.InvalidQuality, err.Error())
	}

	jobType, err := clipdom.ResolveType(req.ClipType, job.Type)
	if err != nil {
		return nil, newRejection(fiber.StatusBadRequest, api.InvalidClipType, err.Error())
	}

	edit := clipEdit(req)

	edit.Type = jobType

	err = handler.validateEdit(ctx.Context(), job.InputPath, edit, jobType)
	if err != nil {
		return nil, newRejection(fiber.StatusBadRequest, api.InvalidRequest, err.Error())
	}

	job.Apply(edit)

	err = handler.db.SaveClip(ctx.Context(), job)
	if err != nil {
		return nil, newRejection(fiber.StatusInternalServerError, api.PersistFailed, err.Error())
	}

	err = handler.maybeRegenerate(ctx, job)
	if err != nil {
		return nil, submitFailure(err)
	}

	return job, nil
}

// respondWithUpdateFailure answers a rejected clip edit.
//
// Parameters:
//   - ctx: Request context.
//   - failure: The rejected edit.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func respondWithUpdateFailure(ctx fiber.Ctx, failure *clipRejection) error {
	if !isBrowserHTMXSubmit(ctx) {
		return respond.WriteError(ctx, failure.status, failure.code, failure.message)
	}

	ctx.Set(routes.HeaderHXReswap, routes.HXTargetNone)

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return flash.OutOfBand(failure.message).Render(ctx.Context(), writer)
	})
}

// newRejection describes a rejected clip edit.
//
// Parameters:
//   - status: HTTP status an API caller is told.
//   - code: Machine-readable API error code.
//   - message: Human-readable error text.
//
// Returns:
//   - failure: The rejection to report.
func newRejection(status int, code api.ErrorCode, message string) *clipRejection {
	return &clipRejection{status: status, code: code, message: message}
}

// isBrowserHTMXSubmit reports whether htmx issued a browser form post.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - True when the request is a browser form submit htmx issued.
func isBrowserHTMXSubmit(ctx fiber.Ctx) bool {
	return respond.IsHTMXRequest(ctx) && respond.IsFormRequest(ctx)
}

// respondWithClipCard answers a saved edit.
//
// Parameters:
//   - ctx: Request context.
//   - job: The clip just saved.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) respondWithClipCard(ctx fiber.Ctx, job *clipdom.Job) error {
	if !respond.IsHTMXRequest(ctx) {
		return respond.RedirectTo(ctx, respond.RefererOrFallback(ctx, ReturnPath(job.MediaID)))
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		item := view.NewClipItem(
			job,
			profile.SelectableProfiles(ctx.Context(), handler.db),
			clipdom.DurationCap(handler.cfg.MaxClipDur),
			library.FileExists(job.OutputPath),
		)

		return clipcard.ClipCard(item).Render(ctx.Context(), writer)
	})
}

// applyRequestQuality resolves a non-empty quality field onto a profile id.
//
// Parameters:
//   - ctx: Request context.
//   - req: Parsed request, updated with the resolved quality.
//
// Returns:
//   - err: Non-nil when the named profile is not recognized.
func (handler *Handler) applyRequestQuality(
	ctx context.Context,
	req *api.ClipRequest,
) error {
	if req.Quality == "" {
		return nil
	}

	quality, err := profile.ResolveProfile(ctx, handler.db, req.Quality)
	if err != nil {
		return fmt.Errorf("apply quality: %w", err)
	}

	req.Quality = quality

	return nil
}

// maybeRegenerate re-queues a clip when the regenerate form flag is set.
//
// Parameters:
//   - ctx: Request context.
//   - job: The clip just saved.
//
// Returns:
//   - err: Non-nil when the queue would not take the clip again.
func (handler *Handler) maybeRegenerate(ctx fiber.Ctx, job *clipdom.Job) error {
	if ctx.FormValue("regenerate") != "1" {
		return nil
	}

	err := handler.queueRegenerate(job)
	if err != nil {
		return fmt.Errorf("regenerate: %w", err)
	}

	return nil
}

// queueRegenerate re-queues a clip after metadata changes.
//
// Parameters:
//   - job: The clip to run again.
//
// Returns:
//   - err: ErrJobActive when the clip is already rendering or queued.
func (handler *Handler) queueRegenerate(job *clipdom.Job) error {
	job.OutputPath = handler.clipPaths.OutputPath(job.ID, job.Type)

	err := handler.clipQueue.Requeue(job)
	if err != nil {
		return fmt.Errorf("requeue: %w", err)
	}

	return nil
}
