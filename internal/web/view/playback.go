// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

// Playback is live Plex playback for a media item.
type Playback struct {
	Playing    bool
	ViewOffset float64
	Title      string
}
