// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"github.com/PapagoLabs/outtake/internal/clip/profile"
)

// ClipProfileItem is one editable clip profile on the settings page.
type ClipProfileItem struct {
	ID        string
	Name      string
	CRF       int
	Preset    string
	AudioKbps int
	MaxWidth  int
	IsDefault bool
	// KeepHDR is the profile's keep-HDR default for new video clips.
	KeepHDR bool
}

// ClipProfileItems maps stored clip profiles onto the settings page models.
//
// Parameters:
//   - stored: Profiles the page lists.
//
// Returns:
//   - items: One page model per profile, in the order the profiles were given.
func ClipProfileItems(stored []profile.Profile) []ClipProfileItem {
	items := make([]ClipProfileItem, 0, len(stored))

	for index := range stored {
		items = append(items, ClipProfileItem{
			ID:        stored[index].ID,
			Name:      stored[index].Name,
			CRF:       stored[index].CRF,
			Preset:    stored[index].Preset,
			AudioKbps: stored[index].AudioKbps,
			MaxWidth:  stored[index].MaxWidth,
			IsDefault: stored[index].IsDefault,
			KeepHDR:   stored[index].KeepHDR,
		})
	}

	return items
}
