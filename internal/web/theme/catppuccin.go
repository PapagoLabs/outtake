// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

const (
	// CatppuccinMauve is Catppuccin mauve on light surfaces.
	catppuccinMauve = "oklch(0.555 0.25 297.016)"

	// CatppuccinMocha is Catppuccin mocha mantle.
	catppuccinMocha = "oklch(0.216 0.025 284.065)"

	// CatppuccinBloom is Catppuccin mauve on dark surfaces.
	catppuccinBloom = "oklch(0.787 0.119 304.769)"
)

// Catppuccin is the Catppuccin palette.
var Catppuccin = Palette{
	ID:      PaletteCatppuccin,
	Name:    "Catppuccin",
	Accent:  "#CBA6F7",
	Surface: "#1E1E2E",
	Light: Scheme{
		Background:        "oklch(0.958 0.006 264.532)",
		Foreground:        "oklch(0.435 0.043 279.325)",
		Card:              "oklch(0.98 0.004 264)",
		Primary:           catppuccinMauve,
		PrimaryForeground: "oklch(0.98 0.004 264)",
		Subtle:            "oklch(0.92 0.01 264)",
		MutedForeground:   "oklch(0.55 0.03 279)",
		Destructive:       "oklch(0.55 0.2 25)",
		Border:            "oklch(0.88 0.012 264)",
		Input:             "oklch(0.88 0.012 264)",
		Ring:              catppuccinMauve,
		Sidebar:           "oklch(0.94 0.008 264)",
		SidebarPrimary:    catppuccinMauve,
		SidebarAccent:     "oklch(0.9 0.012 264)",
	},
	Dark: Scheme{
		Background:        "oklch(0.243 0.03 283.911)",
		Foreground:        "oklch(0.879 0.043 272.277)",
		Card:              catppuccinMocha,
		Primary:           catppuccinBloom,
		PrimaryForeground: catppuccinMocha,
		Subtle:            "oklch(0.324 0.032 281.978)",
		MutedForeground:   "oklch(0.7 0.03 272)",
		Destructive:       "oklch(0.75 0.12 8)",
		Border:            darkHairline,
		Input:             darkWell,
		Ring:              catppuccinBloom,
		Sidebar:           catppuccinMocha,
		SidebarPrimary:    catppuccinBloom,
		SidebarAccent:     "oklch(0.324 0.032 281.978)",
	},
}
