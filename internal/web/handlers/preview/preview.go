// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"fmt"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	clippreview "github.com/PapagoLabs/outtake/internal/clip/preview"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	clips "github.com/PapagoLabs/outtake/internal/web/handlers/clip"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// Handler renders and serves the segment previews the export form shows.
type Handler struct {
	previews *clippreview.Service
	cfg      *config.Config
	sources  *library.MediaSource
}

const (
	// messagePreviewNotRunning explains a cancel that changed nothing.
	messagePreviewNotRunning = "preview is not running"
)

// New returns a preview handler wired to the supplied collaborators.
//
// Parameters:
//   - previews: Preview service the routes submit to.
//   - cfg: Configuration supplying the keep-HDR default.
//   - sources: Resolver for the media a preview is cut from.
//
// Returns:
//   - handler: A ready-to-use preview handler.
func New(
	previews *clippreview.Service,
	cfg *config.Config,
	sources *library.MediaSource,
) *Handler {
	return &Handler{
		previews: previews,
		cfg:      cfg,
		sources:  sources,
	}
}

// CancelPreview stops an in-flight preview render.
//
// Parameters:
//   - ctx: Incoming request.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) CancelPreview(ctx fiber.Ctx) error {
	id := ctx.Params(routes.ParamID)

	// Existence is checked separately from running, so an id nothing was ever
	// registered under is not reported as a render that already finished.
	if _, known := handler.previews.Status(id); !known {
		return respond.WriteNotFound(ctx)
	}

	if !handler.previews.Cancel(id) {
		return respond.WriteError(
			ctx,
			fiber.StatusConflict,
			api.PreviewNotRunning,
			messagePreviewNotRunning,
		)
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, fiber.Map{"id": id})
}

// Preview renders a short low-quality segment without saving a clip.
//
// Parameters:
//   - ctx: Request context carrying the preview fields.
//
// Returns:
//   - err: Response or redirect error, or nil on success.
func (handler *Handler) Preview(ctx fiber.Ctx) error {
	req, err := clips.ParseRequest(ctx)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, api.InvalidRequest, err.Error())
	}

	inputPath, err := handler.sources.Resolve(ctx.Context(), req.MediaID)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, api.MediaPathUnresolved, err.Error())
	}

	// Resolved before the id is derived, because the id has to reflect the
	// setting the render will actually use, not only what the form sent.
	preserveHDR := api.FlagOrDefault(req.PreserveHDR, handler.cfg.PreserveHDR)

	req.PreserveHDR = new(preserveHDR)

	previewID, err := clippreview.RequestID(req, inputPath)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, api.MediaPathUnresolved, err.Error())
	}

	// A preview already rendered for these exact parameters is returned without
	// touching ffmpeg, so repeating the same selection costs nothing. It is
	// recorded as finished so the page it redirects to can poll a status rather
	// than an unknown id.
	if handler.previews.Published(previewID) {
		handler.previews.Remember(previewID)

		return respond.RedirectTo(ctx, previewRedirect(req, previewID))
	}

	// The render detaches from this request, so the values it needs are captured
	// by the closure rather than read from the context after the response has
	// been written. None of them change once the request is parsed.
	//
	// Admission and registration happen together inside the service, so a burst
	// of clicks cannot each observe the same headroom and collectively overshoot
	// the limit.
	admitted := handler.previews.Submit(
		ctx.Context(),
		previewID,
		func(renderCtx context.Context) error {
			return handler.previews.RenderInto(renderCtx, previewID, inputPath, req, preserveHDR)
		},
	)
	if admitted == clippreview.RefusedFull {
		return respond.WriteError(
			ctx,
			fiber.StatusTooManyRequests,
			api.PreviewBusy,
			clippreview.ErrBusy.Error(),
		)
	}

	return respond.RedirectTo(ctx, previewRedirect(req, previewID))
}

// PreviewFile serves a generated segment preview.
//
// Parameters:
//   - ctx: Request for one preview id.
//
// Returns:
//   - err: Non-nil when the id is invalid or the stream fails.
func (handler *Handler) PreviewFile(ctx fiber.Ctx) error {
	id := ctx.Params(routes.ParamID)
	if !clippreview.ValidID(id) {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	err := respond.SendRangedFile(ctx, handler.previews.OutputPath(id))
	if err != nil {
		return fmt.Errorf("send preview: %w", err)
	}

	return nil
}

// PreviewStatus reports how far a preview render has got.
//
// Parameters:
//   - ctx: Incoming request.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) PreviewStatus(ctx fiber.Ctx) error {
	view, ok := handler.previews.Status(ctx.Params(routes.ParamID))
	if !ok {
		return respond.WriteNotFound(ctx)
	}

	payload := fiber.Map{
		"id":       view.ID,
		"status":   view.Status,
		"progress": view.Progress,
	}

	// The file is only served once it is published, so the URL is withheld until
	// then rather than pointing at something that is not there.
	if view.Status == clip.StatusCompleted && handler.previews.Published(view.ID) {
		payload["url"] = routes.PathPreviewPrefix + view.ID
	}

	if view.Error != "" {
		payload["error"] = view.Error
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, payload)
}
