// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"net/url"
	"time"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// SessionItem is one live Plex playback session.
type SessionItem struct {
	ID         string
	MediaID    string
	Parts      []Crumb
	Year       int
	ViewOffset time.Duration
	Duration   time.Duration
}

// SessionItems maps live Plex sessions onto the dashboard models.
//
// Parameters:
//   - sessions: Sessions playing right now.
//
// Returns:
//   - items: One page model per live session, in the order they arrived.
func SessionItems(sessions []plex.Session) []SessionItem {
	items := make([]SessionItem, 0, len(sessions))

	for index := range sessions {
		session := &sessions[index]

		parts, year := SessionTitleParts(session.MediaItem)

		items = append(items, SessionItem{
			ID:         session.ID,
			MediaID:    session.MediaItem.ID,
			Parts:      parts,
			Year:       year,
			ViewOffset: timecode.FromSeconds(session.ViewOffset).Duration(),
			Duration:   timecode.FromSeconds(session.Duration).Duration(),
		})
	}

	return items
}

// ResumeURL is where the Clip now link points, carrying the playback offset.
//
// Returns:
//   - location: Media item URL starting at the offset, or an empty string when
//     the session plays something without a media id.
func (item SessionItem) ResumeURL() string {
	if item.MediaID == "" {
		return ""
	}

	values := url.Values{}
	values.Set(routes.QueryStart, timecode.FromDuration(item.ViewOffset).FormatSeconds())

	return routes.ItemURL(item.MediaID, values)
}
