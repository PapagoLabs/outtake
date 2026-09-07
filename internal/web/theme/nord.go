// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

const (
	// NordPolar is Nord polar night.
	nordPolar = "oklch(0.324 0.023 264.182)"

	// NordFrost is Nord frost accent on light surfaces.
	nordFrost = "oklch(0.594 0.077 254.028)"

	// NordAurora is Nord frost accent on dark surfaces.
	nordAurora = "oklch(0.775 0.062 217.469)"
)

// Nord is the Nord palette.
var Nord = Palette{
	ID:      PaletteNord,
	Name:    "Nord",
	Accent:  "#88C0D0",
	Surface: "#2E3440",
	Light: Scheme{
		Background:        "oklch(0.951 0.007 260.732)",
		Foreground:        nordPolar,
		Card:              "oklch(0.97 0.005 260)",
		Primary:           nordFrost,
		PrimaryForeground: "oklch(0.951 0.007 260.732)",
		Subtle:            "oklch(0.9 0.012 260)",
		MutedForeground:   "oklch(0.45 0.03 264)",
		Destructive:       "oklch(0.577 0.118 19.8)",
		Border:            "oklch(0.85 0.015 260)",
		Input:             "oklch(0.85 0.015 260)",
		Ring:              nordFrost,
		Sidebar:           "oklch(0.92 0.01 260)",
		SidebarPrimary:    nordFrost,
		SidebarAccent:     "oklch(0.88 0.015 260)",
	},
	Dark: Scheme{
		Background:        nordPolar,
		Foreground:        "oklch(0.899 0.016 262.749)",
		Card:              "oklch(0.379 0.029 266.471)",
		Primary:           nordAurora,
		PrimaryForeground: nordPolar,
		Subtle:            "oklch(0.42 0.03 266)",
		MutedForeground:   "oklch(0.72 0.02 260)",
		Destructive:       "oklch(0.63 0.12 20)",
		Border:            darkHairline,
		Input:             darkWell,
		Ring:              nordAurora,
		Sidebar:           "oklch(0.3 0.025 264)",
		SidebarPrimary:    nordAurora,
		SidebarAccent:     "oklch(0.379 0.029 266.471)",
	},
}
