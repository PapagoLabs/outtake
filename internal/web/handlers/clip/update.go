// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"errors"
	"fmt"
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	clipcard "github.com/PapagoLabs/outtake/internal/web/components/clip"
	"github.com/PapagoLabs/outtake/internal/web/components/flash"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// clipRejection is a rejected clip edit, held rather than written.
type clipRejection struct {
	status  int
	code    api.ErrorCode
	failure view.Failure
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

// editableJob finds a clip the caller may change.
//
// The clip must be the caller's, and its source must still be one Plex shares
// with them, because an edit renders it again from the file on disk.
//
// Parameters:
//   - ctx: Request whose caller edits the clip.
//   - id: The clip.
//
// Returns:
//   - job: The clip, or nil when the edit is refused.
//   - rejection: Why the edit is refused, or nil.
func (handler *Handler) editableJob(ctx fiber.Ctx, id string) (*clipdom.Job, *clipRejection) {
	job := handler.lookupJob(ctx.Context(), middleware.ClipScope(ctx), id)
	if job == nil {
		return nil, newRejection(
			fiber.StatusNotFound,
			api.NotFound,
			view.NewNotice(respond.NotFoundMessage),
		)
	}

	err := handler.sources.CheckAccess(ctx.Context(), middleware.PlexViewer(ctx), job.MediaID)
	if errors.Is(err, library.ErrNotShared) {
		return nil, newRejection(fiber.StatusBadRequest, api.NotShared, respond.Fail(ctx, err))
	}

	if err != nil {
		return nil, newRejection(
			fiber.StatusBadGateway,
			api.MediaPathUnresolved,
			respond.Fail(ctx, err),
		)
	}

	return job, nil
}

// stageClipUpdate runs every check, then applies and saves the edit.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - job: The saved clip, or nil when the edit was rejected.
//   - failure: Why the edit was rejected, or nil when it was saved.
func (handler *Handler) stageClipUpdate(ctx fiber.Ctx) (*clipdom.Job, *clipRejection) {
	id := ctx.Params(routes.ParamID)

	job, rejection := handler.editableJob(ctx, id)
	if rejection != nil {
		return nil, rejection
	}

	req, err := parseEdit(ctx)
	if err != nil {
		return nil, newRejection(fiber.StatusBadRequest, api.InvalidRequest, respond.Fail(ctx, err))
	}

	quality, err := handler.resolveEditQuality(ctx.Context(), req.Quality, job.Quality)
	if err != nil {
		return nil, newRejection(fiber.StatusBadRequest, api.InvalidQuality, respond.Fail(ctx, err))
	}

	jobType, err := clipdom.ResolveType(valueOr(req.ClipType, ""), job.Type)
	if err != nil {
		return nil, newRejection(
			fiber.StatusBadRequest,
			api.InvalidClipType,
			respond.Fail(ctx, err),
		)
	}

	edit := mergeEdit(job, req, jobType, quality)

	edit.PreserveHDR = handler.renderedKeepHDR(ctx, job, edit)

	err = handler.validateEdit(ctx.Context(), job.InputPath, edit)
	if err != nil {
		return nil, newRejection(fiber.StatusBadRequest, api.InvalidRequest, respond.Fail(ctx, err))
	}

	saved, err := handler.saveEdit(ctx, id, edit)
	if errors.Is(err, catalog.ErrClipNotFound) {
		return nil, newRejection(
			fiber.StatusNotFound,
			api.NotFound,
			view.NewNotice(respond.NotFoundMessage),
		)
	}

	if err != nil {
		return nil, submitFailure(ctx, err)
	}

	return saved, nil
}

// renderedKeepHDR takes Keep HDR from the edit's profile when the edit renders.
//
// The stored value then always describes the clip's file. An edit renders when
// it regenerates, changes the type, or reaches a clip still waiting to render.
// Any other edit keeps the stored value. So does one whose profile is gone,
// such as a stored profile since deleted, rather than taking the setting of
// [clipdom.DefaultPreset].
//
// Parameters:
//   - ctx: Request context.
//   - job: The stored clip.
//   - edit: The merged edit.
//
// Returns:
//   - keep: The profile's setting when the edit renders and the profile is
//     found, otherwise nil.
func (handler *Handler) renderedKeepHDR(
	ctx fiber.Ctx,
	job *clipdom.Job,
	edit clipdom.Edit,
) *bool {
	renders := regenerates(ctx) || edit.Type != job.Type || job.Status == clipdom.StatusPending
	if !renders {
		return nil
	}

	keep, found := profile.KeepHDR(ctx.Context(), handler.db, edit.Quality)
	if !found {
		return nil
	}

	return &keep
}

// regenerates reports whether the update asked for the clip to render again.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - regenerate: True when the form posted regenerate.
func regenerates(ctx fiber.Ctx) bool {
	return ctx.FormValue("regenerate") == routes.FormChecked
}

// saveEdit applies an edit through the catalog, rendering the clip again when
// the form asked for it.
//
// Parameters:
//   - ctx: Request context.
//   - id: Clip to change.
//   - edit: The validated change.
//
// Returns:
//   - job: The saved clip.
//   - err: The catalog's failure.
func (handler *Handler) saveEdit(
	ctx fiber.Ctx,
	id string,
	edit clipdom.Edit,
) (*clipdom.Job, error) {
	update := catalog.Update
	if regenerates(ctx) {
		update = catalog.UpdateAndRegenerate
	}

	job, err := update(ctx.Context(), handler.clipQueue, handler.db, handler.clipPaths, id, edit)
	if err != nil {
		return nil, fmt.Errorf("save edit: %w", err)
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
		return respond.WriteFailure(ctx, failure.status, failure.code, failure.failure)
	}

	ctx.Set(routes.HeaderHXReswap, routes.HXTargetNone)

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return flash.OutOfBand(failure.failure).Render(ctx.Context(), writer)
	})
}

// newRejection describes a rejected clip edit.
//
// Parameters:
//   - status: HTTP status an API caller is told.
//   - code: Machine-readable API error code.
//   - failure: What the page shows.
//
// Returns:
//   - rejection: The rejection to report.
func newRejection(status int, code api.ErrorCode, failure view.Failure) *clipRejection {
	return &clipRejection{status: status, code: code, failure: failure}
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
			handler.outputExists(ctx.Context())(job.OutputPath),
		)

		item.SDRExists = handler.sdrVersionExists(ctx.Context(), job)

		// The swapped-in card replaces one the page filled in from the source,
		// so it is filled in the same way or it loses the HDR checkbox, the
		// audio tracks, and the source length.
		view.ApplySource(&item, handler.sources.DescribePath(ctx.Context(), job.InputPath))

		return clipcard.ClipCard(item).Render(ctx.Context(), writer)
	})
}
