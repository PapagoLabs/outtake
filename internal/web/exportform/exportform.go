// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package exportform

import (
	"time"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// Window is the New export form's start and end marks.
type Window struct {
	// Start is where the clip begins in the source.
	Start time.Duration
	// End is where the clip ends in the source.
	End time.Duration
}

// defaultSegment is the clip window the media item page offers when the request
// carries no end mark.
const defaultSegment = 10 * time.Second

// FromRequest captures the export form fields of a parsed request.
//
// Parameters:
//   - req: The parsed form or JSON request.
//
// Returns:
//   - form: The export form state to carry across the redirect.
func FromRequest(req api.ClipRequest) view.ExportForm {
	return view.ExportForm{
		Type:          clip.Type(req.ClipType),
		Name:          req.Name,
		Quality:       req.Quality,
		AudioIndex:    req.AudioIndex,
		Width:         req.Width,
		FPS:           req.FPS,
		CropBlackBars: req.CropBlackBars,
	}
}

// FromQuery reads the export form state off the request.
//
// Parameters:
//   - ctx: Incoming page request.
//   - defaultCrop: Configured crop-black-bars default.
//   - title: Media title, used when the request carries no clip name.
//
// Returns:
//   - form: The export form state to render.
func FromQuery(
	ctx fiber.Ctx,
	defaultCrop bool,
	title string,
) view.ExportForm {
	form := view.ExportForm{
		Type:          NormalizeType(ctx.Query(routes.QueryExportType)),
		Name:          title,
		Quality:       ctx.Query(routes.QueryQuality),
		AudioIndex:    respond.QueryInt(ctx, routes.QueryAudioIndex),
		Width:         respond.QueryInt(ctx, routes.QueryWidth),
		FPS:           respond.QueryInt(ctx, routes.QueryFPS),
		CropBlackBars: defaultCrop,
	}

	// A carried name is kept exactly as submitted, including when empty, so
	// clearing the field survives instead of being refilled with the title.
	if carried, ok := ctx.Queries()[routes.QueryExportName]; ok {
		form.Name = carried
	}

	if raw := ctx.Query(routes.QueryCropBlackBars); raw != "" {
		form.CropBlackBars = routes.IsFormChecked(raw)
	}

	return form
}

// MediaItemForm builds the New export form state for the media item page.
//
// Parameters:
//   - ctx: Incoming page request.
//   - cfg: Configuration supplying the crop default.
//   - title: Resolved media title, used when the query carries no clip name.
//
// Returns:
//   - form: The export form state to render.
func MediaItemForm(
	ctx fiber.Ctx,
	cfg *config.Config,
	title string,
) view.ExportForm {
	return FromQuery(ctx, cfg.CropBlackBars, title)
}

// ClipWindow reads the start and end marks of the New export form.
//
// Parameters:
//   - ctx: Incoming page request.
//
// Returns:
//   - window: The marks, with end guaranteed to be ahead of start.
func ClipWindow(ctx fiber.Ctx) Window {
	start, err := timecode.Parse(ctx.Query(routes.QueryStart))
	if err != nil {
		start = timecode.Timecode{}
	}

	mark := start

	end, err := timecode.Parse(ctx.Query(routes.QueryEnd))
	if err != nil || end.Duration() <= start.Duration() {
		end = timecode.FromDuration(mark.Duration() + defaultSegment)
	}

	return Window{Start: mark.Duration(), End: end.Duration()}
}

// NormalizeType maps a query value onto an export type the form offers.
//
// Parameters:
//   - raw: Query value for the export type.
//
// Returns:
//   - kind: A supported export type, or the default when raw is not offered.
func NormalizeType(raw string) clip.Type {
	kind, ok := clip.ParseType(raw)
	if !ok {
		return clip.TypeClip
	}

	return kind
}
