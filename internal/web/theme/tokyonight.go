// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

const (
	// TokyoBlue is Tokyo Night blue on light surfaces.
	tokyoBlue = "oklch(0.48 0.13 264)"

	// TokyoNightBg is Tokyo Night background.
	tokyoNight = "oklch(0.204 0.016 284.91)"

	// TokyoGlow is Tokyo Night blue on dark surfaces.
	tokyoGlow = "oklch(0.719 0.132 264.202)"
)

// TokyoNight is the Tokyo Night palette.
var TokyoNight = Palette{
	ID:      PaletteTokyoNight,
	Name:    "Tokyo Night",
	Accent:  "#7AA2F7",
	Surface: "#1A1B26",
	Light: Scheme{
		Background:        "oklch(0.92 0.01 270)",
		Foreground:        "oklch(0.32 0.04 270)",
		Card:              "oklch(0.96 0.008 270)",
		Primary:           tokyoBlue,
		PrimaryForeground: "oklch(0.96 0.008 270)",
		Subtle:            "oklch(0.88 0.015 270)",
		MutedForeground:   "oklch(0.5 0.03 270)",
		Destructive:       "oklch(0.55 0.16 15)",
		Border:            "oklch(0.84 0.015 270)",
		Input:             "oklch(0.84 0.015 270)",
		Ring:              tokyoBlue,
		Sidebar:           "oklch(0.9 0.012 270)",
		SidebarPrimary:    tokyoBlue,
		SidebarAccent:     "oklch(0.86 0.015 270)",
	},
	Dark: Scheme{
		Background:        "oklch(0.226 0.021 280.487)",
		Foreground:        "oklch(0.846 0.061 274.763)",
		Card:              tokyoNight,
		Primary:           tokyoGlow,
		PrimaryForeground: tokyoNight,
		Subtle:            "oklch(0.306 0.037 273.227)",
		MutedForeground:   "oklch(0.65 0.04 274)",
		Destructive:       "oklch(0.7 0.14 12)",
		Border:            darkHairline,
		Input:             darkWell,
		Ring:              tokyoGlow,
		Sidebar:           tokyoNight,
		SidebarPrimary:    tokyoGlow,
		SidebarAccent:     "oklch(0.306 0.037 273.227)",
	},
}
