// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

import (
	"strings"
)

// Palette is a named color theme with light and dark schemes.
type Palette struct {
	ID      string
	Name    string
	Accent  string
	Surface string
	Light   Scheme
	Dark    Scheme
}

const (
	// PalettePlex is the default Outtake color palette.
	PalettePlex = "plex"

	// PaletteNeutral is the original zinc palette.
	PaletteNeutral = "neutral"

	// PaletteNord is the Nord palette.
	PaletteNord = "nord"

	// PaletteCatppuccin is the Catppuccin palette.
	PaletteCatppuccin = "catppuccin"

	// PaletteDracula is the Dracula palette.
	PaletteDracula = "dracula"

	// PaletteTokyoNight is the Tokyo Night palette.
	PaletteTokyoNight = "tokyo-night"
)

// Palettes returns registered color themes, default first.
func Palettes() []Palette {
	return []Palette{
		Plex,
		Neutral,
		Nord,
		Catppuccin,
		Dracula,
		TokyoNight,
	}
}

// IDList returns palette IDs as a comma-separated html data-palettes value.
func IDList() string {
	palettes := Palettes()
	ids := make([]string, 0, len(palettes))

	for i := range palettes {
		ids = append(ids, palettes[i].ID)
	}

	return strings.Join(ids, ",")
}
