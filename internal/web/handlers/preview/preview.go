// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"errors"
	"fmt"
	"time"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/playback"
	clippreview "github.com/PapagoLabs/outtake/internal/clip/preview"
	clipprofile "github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/clip/quota"
	"github.com/PapagoLabs/outtake/internal/failure"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
	clips "github.com/PapagoLabs/outtake/internal/web/handlers/clip"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	webview "github.com/PapagoLabs/outtake/internal/web/view"
)

// Handler renders and serves the segment previews the export form shows.
type Handler struct {
	previews *clippreview.Service
	cfg      *config.Config
	db       *database.DB
	sources  *library.MediaSource
	// limits admits a member's previews. Without it, only the owner and
	// admins render previews.
	limits *quota.Enforcer
}

const (
	// messagePreviewNotRunning explains a cancel that changed nothing.
	messagePreviewNotRunning = "This preview isn't rendering"

	// messagePreviewGone explains a preview id nothing is registered under.
	messagePreviewGone = "This preview is gone. Select Preview to render it again."
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
	// Another account's preview reads as unknown.
	if _, known := handler.previews.Status(id); !known || !reaches(middleware.ClipScope(ctx), id) {
		return respond.WriteError(ctx, fiber.StatusNotFound, api.NotFound, messagePreviewGone)
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

// Limits returns the enforcer that admits a member's renders.
//
// Returns:
//   - limits: The enforcer, nil until SetLimits sets one.
func (handler *Handler) Limits() *quota.Enforcer {
	return handler.limits
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
		return respond.WriteFailure(
			ctx,
			fiber.StatusBadRequest,
			api.InvalidRequest,
			respond.Fail(ctx, err),
		)
	}

	inputPath, code, err := handler.resolveSelection(ctx, req)
	if err != nil {
		return respond.WriteFailure(ctx, fiber.StatusBadRequest, code, respond.Fail(ctx, err))
	}

	// The id is derived from what the render does, so an SDR screen never
	// gets an HDR preview.
	req.PreserveHDR = new(handler.keepHDR(ctx, req))

	sourceHDR := handler.sources.DescribePath(ctx.Context(), inputPath).HDR
	render := previewRender(req, sourceHDR, screenShowsHDR(ctx))
	maxWidth := playback.MaxPreviewWidth(ctx.Context(), handler.db)

	previewID, err := clippreview.RequestID(
		render,
		inputPath,
		maxWidth,
		middleware.ClipScope(ctx).Owner,
	)
	if err != nil {
		return respond.WriteFailure(
			ctx,
			fiber.StatusBadRequest,
			api.MediaPathUnresolved,
			respond.Fail(ctx, err),
		)
	}

	location := previewRedirect(req, previewID)

	// A preview already rendered for these exact parameters is returned without
	// touching ffmpeg, so repeating the same selection costs nothing. It is
	// recorded as finished so the page it redirects to can poll a status rather
	// than an unknown id.
	if handler.previews.Published(ctx.Context(), previewID) {
		handler.previews.Remember(ctx.Context(), previewID)

		return respond.RedirectTo(ctx, location)
	}

	err = handler.admitRender(ctx, previewID, inputPath, render, maxWidth)
	if err != nil {
		//nolint:wrapcheck // The refusal is written as the response.
		return writeAdmitFailure(ctx, err)
	}

	return respond.RedirectTo(ctx, location)
}

// previewRender decides what a preview renders.
//
// An HDR clip that keeps HDR is tone mapped unless the browser can show an HDR
// preview, because an HDR preview renders black or washed out on an SDR screen
// in some browsers, and a browser without an HEVC decoder cannot play it at
// all. The preview's format badge then names what it shows. The request keeps
// the clip's own choice.
//
// Parameters:
//   - req: Parsed request whose PreserveHDR holds the clip's resolved choice.
//   - sourceHDR: The source carries an HDR transfer.
//   - screenHDR: The browser can show an HDR preview.
//
// Returns:
//   - render: The request the preview renders, a copy of req.
func previewRender(req api.ClipRequest, sourceHDR, screenHDR bool) api.ClipRequest {
	keep := api.Flag(req.PreserveHDR)
	toneMapped := keep && sourceHDR && !screenHDR

	render := req

	render.PreserveHDR = new(keep && !toneMapped)

	return render
}

// screenShowsHDR reports whether the browser said it can show an HDR preview.
//
// That means its screen shows HDR and it decodes HEVC Main 10. The export
// form's script sets the field. Without it, as for an API caller or a browser
// that cannot tell, the screen is taken to be SDR.
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
	if !clippreview.ValidID(id) || !reaches(middleware.ClipScope(ctx), id) {
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
	id := ctx.Params(routes.ParamID)

	view, ok := handler.previews.Status(id)
	if !ok || !reaches(middleware.ClipScope(ctx), id) {
		return respond.WriteError(ctx, fiber.StatusNotFound, api.NotFound, messagePreviewGone)
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, handler.statusPayload(ctx.Context(), view))
}

// SetLimits sets the enforcer that admits a member's previews.
//
// Parameters:
//   - limits: The enforcer.
func (handler *Handler) SetLimits(limits *quota.Enforcer) {
	handler.limits = limits
}

// admitRender submits a preview's render to the service.
//
// The render detaches from this request, so the values it needs are captured
// by the closure rather than read from the context after the response has
// been written. None of them change once the request is parsed. Admission and
// registration happen together inside the service, so a burst of clicks
// cannot each observe the same headroom and collectively overshoot the limit.
//
// Parameters:
//   - ctx: The preview request.
//   - previewID: The id the preview is published under.
//   - inputPath: The resolved source.
//   - render: The request the preview renders.
//   - maxWidth: The widest the preview is scaled.
//
// Returns:
//   - err: A reached limit, or clippreview.ErrBusy when the service is full
//     or closing, which the client can ask again once it has room.
func (handler *Handler) admitRender(
	ctx fiber.Ctx,
	previewID, inputPath string,
	render api.ClipRequest,
	maxWidth int,
) error {
	renderKeepsHDR := api.Flag(render.PreserveHDR)

	// The id names the caller, so a preview still rendering under it is the
	// caller's own, already counted, and asking again starts nothing new.
	if view, known := handler.previews.Status(previewID); known && !view.Done() {
		return nil
	}

	//nolint:wrapcheck // The refusal is answered as it is.
	return handler.limits.AdmitPreview(ctx.Context(), middleware.RenderMember(ctx), func() error {
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
		if admitted == clippreview.RefusedFull || admitted == clippreview.RefusedClosed {
			return clippreview.ErrBusy
		}

		return nil
	})
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

// resolveSelection resolves a preview's source and checks its window.
//
// A preview is held to the same bounds as the clip it previews, so the
// configured cap applies here too and a bad window fails before it renders.
//
// Parameters:
//   - ctx: The preview request, whose caller must be able to see the item.
//   - req: Parsed preview request.
//
// Returns:
//   - inputPath: Resolved source media path.
//   - code: API error code for whatever failed.
//   - err: Non-nil when the preview may not be rendered.
func (handler *Handler) resolveSelection(
	ctx fiber.Ctx,
	req api.ClipRequest,
) (string, api.ErrorCode, error) {
	kind, err := clip.ResolveType(req.ClipType, clip.TypeClip)
	if err != nil {
		return "", api.InvalidClipType, fmt.Errorf("resolve type: %w", err)
	}

	err = handler.sources.CheckAccess(ctx.Context(), middleware.PlexViewer(ctx), req.MediaID)
	if err != nil {
		return "", clips.AccessCode(err), fmt.Errorf("check access: %w", err)
	}

	inputPath, err := handler.sources.Resolve(ctx.Context(), req.MediaID)
	if err != nil {
		return "", api.MediaPathUnresolved, fmt.Errorf("resolve source: %w", err)
	}

	err = handler.sources.CheckEdit(
		ctx.Context(),
		inputPath,
		clips.RequestEdit(req, kind),
		handler.limits.ClipCap(
			ctx.Context(),
			middleware.RenderMember(ctx),
			clip.DurationCap(handler.cfg.MaxClipDur),
		),
	)
	if err != nil {
		return "", api.InvalidRequest, fmt.Errorf("check selection: %w", err)
	}

	return inputPath, "", nil
}

// statusPayload describes a preview render the way its status reports it.
//
// The poll and the page stream both send it.
//
// Parameters:
//   - ctx: Scope for the lookup of a published file.
//   - view: The render.
//
// Returns:
//   - payload: The status fields, with the URL only once the file is
//     published.
func (handler *Handler) statusPayload(ctx context.Context, view clippreview.View) fiber.Map {
	payload := fiber.Map{
		"id":       view.ID,
		"status":   view.Status,
		"progress": view.Progress,
		// barPercent moves between the whole percents progress counts, so
		// the bar keeps moving through a slow encode.
		"barPercent": clip.BarPercent(view.Progress, view.ExactProgress),
	}

	// The file is only served once it is published, so the URL is withheld until
	// then rather than pointing at something that is not there. The poll only
	// looks the file up, and PreviewFile fetches it when it is asked for.
	if view.Status == clip.StatusCompleted && handler.previews.Exists(ctx, view.ID) {
		payload["url"] = routes.PathPreviewPrefix + view.ID
	}

	if view.Error != "" {
		payload["message"] = view.Error
	}

	if view.ErrorDetails != "" {
		payload["details"] = view.ErrorDetails
	}

	if label := view.Format.Label(); label != "" {
		payload["format"] = label
	}

	if remaining := previewRemaining(view); remaining != "" {
		payload["remaining"] = remaining
	}

	return payload
}

// previewRemaining says how long a rendering preview has left.
//
// Parameters:
//   - view: The preview's state.
//
// Returns:
//   - label: Such as "1m 20s left", empty unless the preview is
//     rendering and has run long enough to estimate.
func previewRemaining(view clippreview.View) string {
	if view.Status != clip.StatusProcessing {
		return ""
	}

	return webview.RemainingLabel(view.Started, time.Now(), view.Progress)
}

// reaches reports whether a caller may see a preview.
//
// A preview belongs to the account its id names. One whose id names no
// account, such as a preview from before ids named their owner, is reached
// only by the owner and admins.
//
// Parameters:
//   - scope: Whose previews the caller may see.
//   - previewID: The preview.
//
// Returns:
//   - reached: True when the caller may see the preview.
func reaches(scope clip.Scope, previewID string) bool {
	owner, _ := clippreview.Owner(previewID)

	return scope.Includes(owner)
}

// writeAdmitFailure answers a preview its render was refused.
//
// Parameters:
//   - ctx: The preview request.
//   - err: Why the render was refused.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func writeAdmitFailure(ctx fiber.Ctx, err error) error {
	if quota.IsLimit(err) {
		return respond.WriteFailure(
			ctx,
			fiber.StatusTooManyRequests,
			api.LimitReached,
			respond.Fail(ctx, err),
		)
	}

	if errors.Is(err, clippreview.ErrBusy) {
		return respond.WriteError(
			ctx,
			fiber.StatusTooManyRequests,
			api.PreviewBusy,
			failure.MessageFor(clippreview.ErrBusy),
		)
	}

	return respond.WriteFailure(
		ctx,
		fiber.StatusInternalServerError,
		api.PreviewBusy,
		respond.Fail(ctx, err),
	)
}
