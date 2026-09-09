// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

const (
	// draculaInk is Dracula background ink.
	draculaInk = "oklch(0.288 0.022 277.509)"

	// draculaPurple is Dracula purple on light surfaces.
	draculaPurple = "oklch(0.5 0.16 302)"

	// draculaGlow is Dracula purple on dark surfaces.
	draculaGlow = "oklch(0.742 0.149 301.883)"
)

// Dracula is the Dracula palette.
var Dracula = Palette{
	ID:      PaletteDracula,
	Name:    "Dracula",
	Accent:  "#BD93F9",
	Surface: "#282A36",
	Light: Scheme{
		Background:        "oklch(0.97 0.01 280)",
		Foreground:        draculaInk,
		Card:              white,
		Primary:           draculaPurple,
		PrimaryForeground: "oklch(0.98 0.008 106)",
		Subtle:            "oklch(0.93 0.015 280)",
		MutedForeground:   "oklch(0.5 0.03 278)",
		Destructive:       "oklch(0.58 0.2 25)",
		Border:            "oklch(0.88 0.02 280)",
		Input:             "oklch(0.88 0.02 280)",
		Ring:              draculaPurple,
		Sidebar:           "oklch(0.95 0.012 280)",
		SidebarPrimary:    draculaPurple,
		SidebarAccent:     "oklch(0.91 0.018 280)",
	},
	Dark: Scheme{
		Background:        draculaInk,
		Foreground:        "oklch(0.977 0.008 106.545)",
		Card:              "oklch(0.34 0.025 277)",
		Primary:           draculaGlow,
		PrimaryForeground: draculaInk,
		Subtle:            "oklch(0.403 0.032 277.832)",
		MutedForeground:   "oklch(0.7 0.04 278)",
		Destructive:       "oklch(0.68 0.19 25)",
		Border:            "oklch(1 0 0 / 12%)",
		Input:             "oklch(1 0 0 / 16%)",
		Ring:              draculaGlow,
		Sidebar:           "oklch(0.26 0.02 277)",
		SidebarPrimary:    draculaGlow,
		SidebarAccent:     "oklch(0.403 0.032 277.832)",
	},
}
