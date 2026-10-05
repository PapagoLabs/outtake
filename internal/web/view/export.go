// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"net/url"
	"strconv"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// ExportForm is the New export form state on the media item page.
type ExportForm struct {
	// Type is the export kind: clip, gif, or screenshot.
	Type clip.Type
	// Name is the user-supplied clip name.
	Name string
	// Quality is the selected clip profile id.
	Quality string
	// AudioIndex is the selected audio stream index on the source.
	AudioIndex int
	// Width is the GIF export width in pixels.
	Width int
	// FPS is the GIF export frame rate.
	FPS int
	// CropBlackBars is the trim-black-bars toggle.
	CropBlackBars bool
	// WebSafeColor is the HDR tone-map toggle.
	WebSafeColor bool
	// PreserveHDR is the keep-HDR-as-is toggle.
	PreserveHDR bool
}

// ApplyToQuery writes the export form state onto a query string, so a redirect
// can carry the form as it was submitted.
//
// Parameters:
//   - values: Query values to mutate.
func (form ExportForm) ApplyToQuery(values url.Values) {
	if form.Type != "" {
		values.Set(routes.QueryExportType, string(form.Type))
	}

	values.Set(routes.QueryExportName, form.Name)

	if form.Quality != "" {
		values.Set(routes.QueryQuality, form.Quality)
	}

	if form.AudioIndex != 0 {
		values.Set(routes.QueryAudioIndex, strconv.Itoa(form.AudioIndex))
	}

	if form.Width != 0 {
		values.Set(routes.QueryWidth, strconv.Itoa(form.Width))
	}

	if form.FPS != 0 {
		values.Set(routes.QueryFPS, strconv.Itoa(form.FPS))
	}

	values.Set(routes.QueryCropBlackBars, routes.CheckedValue(form.CropBlackBars))
	values.Set(routes.QueryWebSafeColor, routes.CheckedValue(form.WebSafeColor))
	values.Set(routes.QueryPreserveHDR, routes.CheckedValue(form.PreserveHDR))
}
