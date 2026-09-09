// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

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

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// AuthStatusResponse represents the authentication status response.
type AuthStatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	UserID        int    `json:"userId,omitempty"`
	Username      string `json:"username,omitempty"`
}
