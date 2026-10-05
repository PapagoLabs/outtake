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
	Error   ErrorCode `json:"error"`
	Message string    `json:"message"`
}

// ErrorCode is the machine-readable code an ErrorResponse carries.
type ErrorCode string

// The codes an ErrorResponse may carry.
const (
	// InvalidRequest reports a request that could not be read or bound.
	InvalidRequest ErrorCode = "invalid_request"

	// NotFound reports a resource nothing is registered under.
	NotFound ErrorCode = "not_found"

	// PersistFailed reports something that could not be saved.
	PersistFailed ErrorCode = "persist_failed"

	// JobActive reports a clip already queued or rendering.
	JobActive ErrorCode = "job_active"

	// MediaPathUnresolved reports a source that could not be resolved.
	MediaPathUnresolved ErrorCode = "media_path"

	// InvalidQuality reports a profile id that names no profile.
	InvalidQuality ErrorCode = "invalid_quality"

	// InvalidClipType reports a clip type the job cannot become.
	InvalidClipType ErrorCode = "invalid_clip_type"

	// NotCancellable reports a clip that is not queued or rendering.
	NotCancellable ErrorCode = "not_cancellable"

	// DeleteFailed reports a clip that could not be removed.
	DeleteFailed ErrorCode = "delete_failed"

	// NotReady reports a clip that has not finished rendering.
	NotReady ErrorCode = "not_ready"

	// FileMissing reports a finished clip whose output is gone.
	FileMissing ErrorCode = "file_missing"

	// PreviewBusy reports a preview refused because every slot is taken.
	PreviewBusy ErrorCode = "preview_busy"

	// PreviewNotRunning reports a preview that was not rendering.
	PreviewNotRunning ErrorCode = "preview_not_running"

	// MissingQuery reports a Plex search carried no query.
	MissingQuery ErrorCode = "missing_query"

	// SearchFailed reports a Plex search that could not be completed.
	SearchFailed ErrorCode = "search_failed"

	// HTTPError reports a request that failed outside a handler's own codes.
	HTTPError ErrorCode = "http_error"

	// PINFailed reports a Plex PIN that could not be created.
	PINFailed ErrorCode = "pin_failed"

	// LogoutFailed reports credentials that could not be cleared.
	LogoutFailed ErrorCode = "logout_failed"
)
