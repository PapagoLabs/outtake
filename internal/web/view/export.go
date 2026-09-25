// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

// ExportForm is the New export form state on the media item page.
//
// A preview redirect replaces the page, so anything not carried across the
// redirect is rebuilt from its default. Grouping the fields keeps that state
// in one place instead of spreading it across the page props.
type ExportForm struct {
	// Type is the export kind: clip, gif, or screenshot.
	Type string
	// Name is the user-supplied clip name.
	Name string
	// Quality is the selected clip profile id.
	Quality string
	// AudioIndex is the selected audio stream index on the source.
	AudioIndex int
	// Width is the GIF export width in pixels. Zero means the form default.
	Width int
	// FPS is the GIF export frame rate. Zero means the form default.
	FPS int
	// CropBlackBars is the trim-black-bars toggle.
	CropBlackBars bool
	// WebSafeColor is the HDR tone-map toggle.
	WebSafeColor bool
}
