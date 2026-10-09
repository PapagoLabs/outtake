// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"fmt"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/playback"
	clippreview "github.com/PapagoLabs/outtake/internal/clip/preview"
	clipprofile "github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
	clips "github.com/PapagoLabs/outtake/internal/web/handlers/clip"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// Handler renders and serves the segment previews the export form shows.
type Handler struct {
	previews *clippreview.Service
	cfg      *config.Config
	db       *database.DB
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
//   - cfg: Configuration supplying the clip length cap.
//   - db: Clip profile store, whose profiles supply the keep-HDR default. It
//     may be nil, which falls back to [clip.DefaultPreset].
//   - sources: Resolver for the media a preview is cut from.
//
// Returns:
//   - handler: A ready-to-use preview handler.
func New(
	previews *clippreview.Service,
	cfg *config.Config,
	db *database.DB,
	sources *library.MediaSource,
) *Handler {
	return &Handler{
		previews: previews,
		cfg:      cfg,
		db:       db,
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

	inputPath, code, err := handler.resolveSelection(ctx.Context(), req)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, code, err.Error())
	}

	// The id is derived from what the render does, so an SDR screen never
	// gets an HDR preview.
	req.PreserveHDR = new(handler.keepHDR(ctx, req))

	sourceHDR := handler.sources.DescribePath(ctx.Context(), inputPath).HDR
	render, shownSDR := previewRender(req, sourceHDR, screenShowsHDR(ctx))
	renderKeepsHDR := api.Flag(render.PreserveHDR)
	maxWidth := playback.MaxPreviewWidth(ctx.Context(), handler.db)

	previewID, err := clippreview.RequestID(render, inputPath, maxWidth)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, api.MediaPathUnresolved, err.Error())
	}

	location := previewRedirect(req, previewID)
	if shownSDR {
		location = shownInSDR(location)
	}

	// A preview already rendered for these exact parameters is returned without
	// touching ffmpeg, so repeating the same selection costs nothing. It is
	// recorded as finished so the page it redirects to can poll a status rather
	// than an unknown id.
	if handler.previews.Published(ctx.Context(), previewID) {
		handler.previews.Remember(ctx.Context(), previewID)

		return respond.RedirectTo(ctx, location)
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
			return handler.previews.RenderInto(
				renderCtx,
				previewID,
				inputPath,
				render,
				renderKeepsHDR,
				maxWidth,
			)
		},
	)
	// A service that is closing refuses like a full one, and the client can
	// ask again once the server is back.
	if admitted == clippreview.RefusedFull || admitted == clippreview.RefusedClosed {
		return respond.WriteError(
			ctx,
			fiber.StatusTooManyRequests,
			api.PreviewBusy,
			clippreview.ErrBusy.Error(),
		)
	}

	return respond.RedirectTo(ctx, location)
}

// previewRender decides what a preview renders. An HDR clip that keeps HDR is
// tone mapped unless the browser can show an HDR preview, because an HDR
// preview renders black or washed out on an SDR screen in some browsers, and
// a browser without an HEVC decoder cannot play it at all. An SDR source has no HDR to show, so its
// preview is never marked as shown in SDR. The request keeps the clip's own
// choice.
//
// Parameters:
//   - req: Parsed request whose PreserveHDR holds the clip's resolved choice.
//   - sourceHDR: The source carries an HDR transfer.
//   - screenHDR: The browser can show an HDR preview.
//
// Returns:
//   - render: The request the preview renders, a copy of req.
//   - shownSDR: The clip keeps HDR but its preview is tone mapped.
func previewRender(req api.ClipRequest, sourceHDR, screenHDR bool) (api.ClipRequest, bool) {
	keep := api.Flag(req.PreserveHDR)
	shownSDR := keep && sourceHDR && !screenHDR

	render := req

	render.PreserveHDR = new(keep && !shownSDR)

	return render, shownSDR
}

// screenShowsHDR reports whether the browser said it can show an HDR preview:
// its screen shows HDR and it decodes HEVC Main 10. The export form's script
// sets the field. Without it, as for an API caller or a
// browser that cannot tell, the screen is taken to be SDR.
//
// Parameters:
//   - ctx: Preview request carrying the form field or query parameter.
//
// Returns:
//   - hdr: True when the request marked the browser as able to show HDR.
func screenShowsHDR(ctx fiber.Ctx) bool {
	value := ctx.FormValue(routes.QueryScreenHDR)
	if value == "" {
		value = ctx.Query(routes.QueryScreenHDR)
	}

	return routes.IsFormChecked(value)
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

	// A preview another instance published is fetched once it is asked for.
	if !handler.previews.Published(ctx.Context(), id) {
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
	// then rather than pointing at something that is not there. The poll only
	// looks the file up, and PreviewFile fetches it when it is asked for.
	if view.Status == clip.StatusCompleted && handler.previews.Exists(ctx.Context(), view.ID) {
		payload["url"] = routes.PathPreviewPrefix + view.ID
	}

	if view.Error != "" {
		payload["error"] = view.Error
	}

	if label := view.Format.Label(); label != "" {
		payload["format"] = label
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, payload)
}

// keepHDR reports whether the clip keeps HDR, which is its profile's setting.
//
// Parameters:
//   - ctx: Preview request.
//   - req: Parsed request whose quality names the profile.
//
// Returns:
//   - keep: True when the profile keeps HDR.
func (handler *Handler) keepHDR(ctx fiber.Ctx, req api.ClipRequest) bool {
	return clipprofile.Preset(ctx.Context(), handler.db, req.Quality).PreserveHDR
}

// resolveSelection resolves a preview's source and checks its window. A
// preview is held to the same bounds as the clip it previews, so the
// configured cap applies here too and a bad window fails before it renders.
//
// Parameters:
//   - ctx: Request context.
//   - req: Parsed preview request.
//
// Returns:
//   - inputPath: Resolved source media path.
//   - code: API error code for whatever failed.
//   - err: Non-nil when the preview may not be rendered.
func (handler *Handler) resolveSelection(
	ctx context.Context,
	req api.ClipRequest,
) (string, api.ErrorCode, error) {
	kind, err := clip.ResolveType(req.ClipType, clip.TypeClip)
	if err != nil {
		return "", api.InvalidClipType, fmt.Errorf("resolve type: %w", err)
	}

	inputPath, err := handler.sources.Resolve(ctx, req.MediaID)
	if err != nil {
		return "", api.MediaPathUnresolved, fmt.Errorf("resolve source: %w", err)
	}

	err = handler.sources.CheckEdit(
		ctx,
		inputPath,
		clips.RequestEdit(req, kind),
		clip.DurationCap(handler.cfg.MaxClipDur),
	)
	if err != nil {
		return "", api.InvalidRequest, fmt.Errorf("check selection: %w", err)
	}

	return inputPath, "", nil
}
