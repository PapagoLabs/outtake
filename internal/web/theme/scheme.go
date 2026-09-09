// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

import (
	"slices"
)

// Scheme is the distinct colors for one appearance of a palette.
type Scheme struct {
	Background        string
	Foreground        string
	Card              string
	Primary           string
	PrimaryForeground string
	Subtle            string
	MutedForeground   string
	Destructive       string
	Border            string
	Input             string
	Ring              string
	Sidebar           string
	SidebarPrimary    string
	SidebarAccent     string
}

const (
	// white is an opaque white surface.
	white = "oklch(1 0 0)"

	// gray97 is a near-white fill.
	gray97 = "oklch(0.97 0 0)"

	// gray922 is a light hairline.
	gray922 = "oklch(0.922 0 0)"

	// darkHairline is a 10% white overlay used on dark surfaces.
	darkHairline = "oklch(1 0 0 / 10%)"

	// darkWell is a 15% white overlay used on dark inputs.
	darkWell = "oklch(1 0 0 / 15%)"
)

// complete reports whether every scheme color is set.
//
// Returns:
//   - ok: True when every scheme color is set.
func (scheme *Scheme) complete() bool {
	fields := []string{
		scheme.Background,
		scheme.Foreground,
		scheme.Card,
		scheme.Primary,
		scheme.PrimaryForeground,
		scheme.Subtle,
		scheme.MutedForeground,
		scheme.Destructive,
		scheme.Border,
		scheme.Input,
		scheme.Ring,
		scheme.Sidebar,
		scheme.SidebarPrimary,
		scheme.SidebarAccent,
	}

	return !slices.Contains(fields, "")
}

// tokens expands a scheme into the CSS variables the UI uses.
//
// Returns:
//   - tokens: The tokens.
func (scheme *Scheme) tokens() Tokens {
	return Tokens{
		Background:               scheme.Background,
		Foreground:               scheme.Foreground,
		Card:                     scheme.Card,
		CardForeground:           scheme.Foreground,
		Popover:                  scheme.Card,
		PopoverForeground:        scheme.Foreground,
		Primary:                  scheme.Primary,
		PrimaryForeground:        scheme.PrimaryForeground,
		Secondary:                scheme.Subtle,
		SecondaryForeground:      scheme.Foreground,
		Muted:                    scheme.Subtle,
		MutedForeground:          scheme.MutedForeground,
		Accent:                   scheme.Subtle,
		AccentForeground:         scheme.Foreground,
		Destructive:              scheme.Destructive,
		Border:                   scheme.Border,
		Input:                    scheme.Input,
		Ring:                     scheme.Ring,
		Sidebar:                  scheme.Sidebar,
		SidebarForeground:        scheme.Foreground,
		SidebarPrimary:           scheme.SidebarPrimary,
		SidebarPrimaryForeground: scheme.PrimaryForeground,
		SidebarAccent:            scheme.SidebarAccent,
		SidebarAccentForeground:  scheme.Foreground,
		SidebarBorder:            scheme.Border,
		SidebarRing:              scheme.Ring,
	}
}
