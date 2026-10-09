// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"time"
)

// Playback is live Plex playback for a media item.
type Playback struct {
	// Playing reports a Plex session on the item.
	Playing bool
	// Paused reports a paused client, whose position is exact.
	Paused bool
	// ViewOffset is the position Plex last reported.
	ViewOffset time.Duration
	// Title is the session's title.
	Title string
	// PositionURL is where the mark buttons read the position fresh.
	PositionURL string
}
