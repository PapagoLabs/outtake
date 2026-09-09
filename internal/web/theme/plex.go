// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

const (
	// plexGold is the Plex accent.
	plexGold = "oklch(0.754 0.155 77.297)"

	// plexInk is Plex body text on light surfaces.
	plexInk = "oklch(0.222 0 0)"
)

// Plex is the default Outtake palette.
var Plex = Palette{
	ID:      PalettePlex,
	Name:    "Plex",
	Accent:  "#E5A00D",
	Surface: "#1B1B1B",
	Light: Scheme{
		Background:        "oklch(0.985 0.005 80)",
		Foreground:        plexInk,
		Card:              white,
		Primary:           plexGold,
		PrimaryForeground: plexInk,
		Subtle:            "oklch(0.96 0.012 80)",
		MutedForeground:   "oklch(0.55 0 0)",
		Destructive:       "oklch(0.577 0.245 27.325)",
		Border:            "oklch(0.91 0.012 80)",
		Input:             "oklch(0.91 0.012 80)",
		Ring:              plexGold,
		Sidebar:           "oklch(0.975 0.008 80)",
		SidebarPrimary:    plexGold,
		SidebarAccent:     "oklch(0.96 0.012 80)",
	},
	Dark: Scheme{
		Background:        plexInk,
		Foreground:        gray97,
		Card:              "oklch(0.285 0 0)",
		Primary:           plexGold,
		PrimaryForeground: plexInk,
		Subtle:            "oklch(0.32 0 0)",
		MutedForeground:   "oklch(0.715 0 0)",
		Destructive:       "oklch(0.704 0.191 22.216)",
		Border:            darkHairline,
		Input:             darkWell,
		Ring:              plexGold,
		Sidebar:           "oklch(0.26 0 0)",
		SidebarPrimary:    plexGold,
		SidebarAccent:     "oklch(0.32 0 0)",
	},
}
