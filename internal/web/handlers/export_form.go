// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"net/url"
	"strconv"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// exportFormState is the New export form state carried across a preview
// redirect.
//
// A preview round trip replaces the page, so every field the redirect does not
// carry is rebuilt from its default. That silently discards a chosen profile,
// audio track, GIF size, export type, or crop toggle between passes, which is
// destructive when the user is mid-way through an edit.
type exportFormState struct {
	Type          string
	Name          string
	Quality       string
	AudioIndex    int
	Width         int
	FPS           int
	CropBlackBars bool
}

// mediaItemWindow is the New export form's start and end marks.
type mediaItemWindow struct {
	Start float64
	End   float64
}

// exportTypes are the export kinds the New export form offers.
var exportTypes = []string{clipTypeClip, clipTypeGIF, clipTypeScreenshot}

// exportFormFromRequest captures the export form fields of a parsed request.
//
// Only the state is taken. Start, end, and the web-safe toggle travel beside
// it because the preview redirect already carries those.
//
// Parameters:
//   - req: The parsed form or JSON request.
//
// Returns:
//   - state: The export form state to carry across the redirect.
func exportFormFromRequest(req api.ClipRequest) exportFormState {
	return exportFormState{
		Type:          req.ClipType,
		Name:          req.Name,
		Quality:       req.Quality,
		AudioIndex:    req.AudioIndex,
		Width:         req.Width,
		FPS:           req.FPS,
		CropBlackBars: req.CropBlackBars,
	}
}

// applyToQuery writes the export form state onto a query string.
//
// Numeric fields are only written when set, because zero means "unset" to the
// GIF bounds check and must keep meaning that after a round trip. The clip
// name and the crop toggle are always written, including when empty or false,
// so a deliberate clear is preserved and stays distinguishable from the query
// omitting the field.
//
// Parameters:
//   - values: Query values to mutate.
func (state exportFormState) applyToQuery(values url.Values) {
	if state.Type != "" {
		values.Set(queryExportType, state.Type)
	}

	values.Set(queryExportName, state.Name)

	if state.Quality != "" {
		values.Set(queryQuality, state.Quality)
	}

	if state.AudioIndex != 0 {
		values.Set(queryAudioIndex, strconv.Itoa(state.AudioIndex))
	}

	if state.Width != 0 {
		values.Set(queryWidth, strconv.Itoa(state.Width))
	}

	if state.FPS != 0 {
		values.Set(queryFPS, strconv.Itoa(state.FPS))
	}

	if state.CropBlackBars {
		values.Set(queryCropBlackBars, formChecked)
	} else {
		values.Set(queryCropBlackBars, queryUnchecked)
	}
}

// viewProps maps the carried state onto the page view model.
//
// Returns:
//   - form: The export form state for the page.
func (state exportFormState) viewProps() view.ExportForm {
	return view.ExportForm{
		Type:          state.Type,
		Name:          state.Name,
		Quality:       state.Quality,
		AudioIndex:    state.AudioIndex,
		Width:         state.Width,
		FPS:           state.FPS,
		CropBlackBars: state.CropBlackBars,
	}
}

// exportFormFromQuery reads the carried export form state back off the query.
//
// An absent crop toggle falls back to the configured default, and an absent
// clip name falls back to the media title, because those are the states a first
// visit to the page should show. Every other field falls back to the form's own
// default, which the template applies.
//
// Parameters:
//   - ctx: Incoming page request.
//   - defaultCrop: Configured crop-black-bars default.
//   - title: Media title, used when the query carries no clip name.
//
// Returns:
//   - state: The export form state to render.
func exportFormFromQuery(ctx fiber.Ctx, defaultCrop bool, title string) exportFormState {
	state := exportFormState{
		Type:          normalizeExportType(ctx.Query(queryExportType)),
		Name:          title,
		Quality:       ctx.Query(queryQuality),
		AudioIndex:    queryInt(ctx, queryAudioIndex),
		Width:         queryInt(ctx, queryWidth),
		FPS:           queryInt(ctx, queryFPS),
		CropBlackBars: defaultCrop,
	}

	// A carried name is kept exactly as submitted, including when empty, so
	// clearing the field survives instead of being refilled with the title.
	if carried, ok := ctx.Queries()[queryExportName]; ok {
		state.Name = carried
	}

	if raw := ctx.Query(queryCropBlackBars); raw != "" {
		state.CropBlackBars = raw == formChecked
	}

	return state
}

// normalizeExportType maps a query value onto an export type the form offers.
//
// Parameters:
//   - raw: Query value for the export type.
//
// Returns:
//   - kind: A supported export type, or the default when raw is not offered.
func normalizeExportType(raw string) string {
	for _, kind := range exportTypes {
		if raw == kind {
			return kind
		}
	}

	return clipTypeClip
}

// mediaItemClipWindow reads the start and end marks of the New export form.
//
// A preview redirect always carries both. A first visit carries neither, and
// an end that is missing or not ahead of start falls back to a default window
// so the form always opens on a usable range.
//
// Parameters:
//   - ctx: Incoming page request.
//
// Returns:
//   - window: The marks, with end guaranteed to be ahead of start.
func mediaItemClipWindow(ctx fiber.Ctx) mediaItemWindow {
	start, err := strconv.ParseFloat(ctx.Query(queryStart), floatBitSize)
	if err != nil {
		start = 0
	}

	end, err := strconv.ParseFloat(ctx.Query(queryEnd), floatBitSize)
	if err != nil || end <= start {
		end = start + defaultSegmentSecs
	}

	return mediaItemWindow{Start: start, End: end}
}

// mediaItemExportForm builds the New export form state for the media item page.
//
// Parameters:
//   - ctx: Incoming page request.
//   - title: Resolved media title, used when the query carries no clip name.
//
// Returns:
//   - form: The export form state to render.
func (handler *HTMLHandler) mediaItemExportForm(ctx fiber.Ctx, title string) view.ExportForm {
	form := exportFormFromQuery(ctx, handler.cfg.CropBlackBars, title).viewProps()

	form.WebSafeColor = previewWebSafeColor(ctx, handler.cfg.WebSafeColor)

	return form
}

// queryInt reads an integer query parameter, treating a bad value as unset.
//
// Parameters:
//   - ctx: Incoming page request.
//   - name: Query parameter name.
//
// Returns:
//   - value: The parsed integer, or zero when absent or malformed.
func queryInt(ctx fiber.Ctx, name string) int {
	value, err := strconv.Atoi(ctx.Query(name))
	if err != nil {
		return 0
	}

	return value
}
