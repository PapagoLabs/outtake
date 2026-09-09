// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"github.com/PapagoLabs/outtake/internal/plex"
	plextitle "github.com/PapagoLabs/outtake/internal/plex/title"
)

// MediaItemResponse represents a media item in API responses.
type MediaItemResponse struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Type         string  `json:"type"`
	Duration     float64 `json:"duration,omitempty"`
	ThumbPath    string  `json:"thumbPath,omitempty"`
	LibraryTitle string  `json:"libraryTitle,omitempty"`
	Year         int     `json:"year,omitempty"`
	Season       int     `json:"season,omitempty"`
	Episode      int     `json:"episode,omitempty"`
	ShowTitle    string  `json:"showTitle,omitempty"`
}

// MediaListResponse represents a paginated list of media items.
type MediaListResponse struct {
	Items []MediaItemResponse `json:"items"`
	Total int                 `json:"total"`
}

// SessionResponse represents a session in API responses.
type SessionResponse struct {
	ID         string  `json:"id"`
	MediaID    string  `json:"mediaId,omitempty"`
	Title      string  `json:"title"`
	Duration   float64 `json:"duration"`
	ViewOffset float64 `json:"viewOffset,omitempty"`
}

// SessionResponses maps Plex sessions onto API payloads.
func SessionResponses(sessions []plex.Session) []SessionResponse {
	responses := make([]SessionResponse, 0, len(sessions))

	for index := range sessions {
		sess := &sessions[index]

		responses = append(responses, SessionResponse{
			ID:         sess.ID,
			MediaID:    sess.MediaItem.ID,
			Title:      plextitle.Display(sess.MediaItem),
			Duration:   sess.Duration,
			ViewOffset: sess.ViewOffset,
		})
	}

	return responses
}
