// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/plex"
)

// ItemResponses maps Plex media items onto API payloads.
//
// Parameters:
//   - items: Media items from a search or a listing.
//
// Returns:
//   - responses: One payload per item, in the order given.
func ItemResponses(items []plex.MediaItem) []api.MediaItemResponse {
	responses := make([]api.MediaItemResponse, 0, len(items))

	for i := range items {
		responses = append(responses, itemResponse(items[i]))
	}

	return responses
}

// SessionResponses maps Plex sessions onto API payloads.
//
// Parameters:
//   - sessions: Sessions from the binding or from a live PMS poll.
//
// Returns:
//   - responses: One payload per session, in the order given.
func SessionResponses(sessions []plex.Session) []api.SessionResponse {
	responses := make([]api.SessionResponse, 0, len(sessions))

	for i := range sessions {
		sess := &sessions[i]

		responses = append(responses, api.SessionResponse{
			ID:         sess.ID,
			MediaID:    sess.MediaItem.ID,
			Title:      sess.MediaItem.DisplayTitle(),
			Duration:   sess.Duration,
			ViewOffset: sess.ViewOffset,
		})
	}

	return responses
}

// itemResponse maps one Plex media item onto its API payload.
//
// Parameters:
//   - item: Media item to describe.
//
// Returns:
//   - response: The item as the API reports it.
func itemResponse(item plex.MediaItem) api.MediaItemResponse {
	return api.MediaItemResponse{
		ID:           item.ID,
		Title:        item.DisplayTitle(),
		Type:         item.Type,
		Duration:     item.Duration,
		ThumbPath:    item.ThumbPath,
		LibraryTitle: item.LibraryTitle,
		Year:         item.Year,
		Season:       item.ParentIndex,
		Episode:      item.Index,
		ShowTitle:    item.GrandparentTitle,
	}
}
