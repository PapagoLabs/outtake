// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

const (
	// NeutralInk is Neutral primary on light surfaces.
	neutralInk = "oklch(0.205 0 0)"

	// NeutralSnow is Neutral near-white text and chrome.
	neutralSnow = "oklch(0.985 0 0)"

	// NeutralNight is Neutral canvas on dark surfaces.
	neutralNight = "oklch(0.145 0 0)"
)

// Neutral is the original zinc palette.
var Neutral = Palette{
	ID:      PaletteNeutral,
	Name:    "Neutral",
	Accent:  "#E5E5E5",
	Surface: "#0A0A0A",
	Light: Scheme{
		Background:        white,
		Foreground:        neutralNight,
		Card:              white,
		Primary:           neutralInk,
		PrimaryForeground: neutralSnow,
		Subtle:            gray97,
		MutedForeground:   "oklch(0.556 0 0)",
		Destructive:       "oklch(0.577 0.245 27.325)",
		Border:            gray922,
		Input:             gray922,
		Ring:              "oklch(0.708 0 0)",
		Sidebar:           neutralSnow,
		SidebarPrimary:    neutralInk,
		SidebarAccent:     gray97,
	},
	Dark: Scheme{
		Background:        neutralNight,
		Foreground:        neutralSnow,
		Card:              neutralInk,
		Primary:           gray922,
		PrimaryForeground: neutralInk,
		Subtle:            "oklch(0.269 0 0)",
		MutedForeground:   "oklch(0.708 0 0)",
		Destructive:       "oklch(0.704 0.191 22.216)",
		Border:            darkHairline,
		Input:             darkWell,
		Ring:              "oklch(0.556 0 0)",
		Sidebar:           neutralInk,
		SidebarPrimary:    "oklch(0.488 0.243 264.376)",
		SidebarAccent:     "oklch(0.269 0 0)",
	},
}
