// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

// PlaybackPosition is where Plex is in one media item when it was asked.
type PlaybackPosition struct {
	// Playing reports a Plex session on the item. Without one the other
	// fields are zero.
	Playing bool `json:"playing"`
	// Paused reports a paused client, whose position is exact.
	Paused bool `json:"paused"`
	// Offset is the position in seconds.
	Offset float64 `json:"offset"`
}
