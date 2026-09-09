// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

// Tokens is the CSS custom properties for one appearance.
type Tokens struct {
	Background               string
	Foreground               string
	Card                     string
	CardForeground           string
	Popover                  string
	PopoverForeground        string
	Primary                  string
	PrimaryForeground        string
	Secondary                string
	SecondaryForeground      string
	Muted                    string
	MutedForeground          string
	Accent                   string
	AccentForeground         string
	Destructive              string
	Border                   string
	Input                    string
	Ring                     string
	Sidebar                  string
	SidebarForeground        string
	SidebarPrimary           string
	SidebarPrimaryForeground string
	SidebarAccent            string
	SidebarAccentForeground  string
	SidebarBorder            string
	SidebarRing              string
}

// pairs returns CSS variable names and values in emit order.
//
// Returns:
//   - items: The CSS variable names and values in emit order.
func (tokens *Tokens) pairs() [][2]string {
	return [][2]string{
		{"background", tokens.Background},
		{"foreground", tokens.Foreground},
		{"card", tokens.Card},
		{"card-foreground", tokens.CardForeground},
		{"popover", tokens.Popover},
		{"popover-foreground", tokens.PopoverForeground},
		{"primary", tokens.Primary},
		{"primary-foreground", tokens.PrimaryForeground},
		{"secondary", tokens.Secondary},
		{"secondary-foreground", tokens.SecondaryForeground},
		{"muted", tokens.Muted},
		{"muted-foreground", tokens.MutedForeground},
		{"accent", tokens.Accent},
		{"accent-foreground", tokens.AccentForeground},
		{"destructive", tokens.Destructive},
		{"border", tokens.Border},
		{"input", tokens.Input},
		{"ring", tokens.Ring},
		{"sidebar", tokens.Sidebar},
		{"sidebar-foreground", tokens.SidebarForeground},
		{"sidebar-primary", tokens.SidebarPrimary},
		{"sidebar-primary-foreground", tokens.SidebarPrimaryForeground},
		{"sidebar-accent", tokens.SidebarAccent},
		{"sidebar-accent-foreground", tokens.SidebarAccentForeground},
		{"sidebar-border", tokens.SidebarBorder},
		{"sidebar-ring", tokens.SidebarRing},
	}
}
