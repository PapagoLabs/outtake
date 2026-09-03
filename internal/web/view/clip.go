// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

// ClipItem is one clip card on the clips list and media item pages.
type ClipItem struct {
	ID            string
	Name          string
	MediaID       string
	MediaTitle    string
	ClipType      string
	Status        string
	Progress      int
	CreatedAt     string
	StartTime     float64
	Duration      float64
	Quality       string
	ProfileName   string
	Profiles      []ClipProfileOption
	FileExists    bool
	AudioIndex    int
	AudioTracks   []AudioTrackOption
	CropBlackBars bool
	Error         string
}

// ClipProfileOption is a named encode profile in a quality select.
type ClipProfileOption struct {
	ID        string
	Name      string
	IsDefault bool
}

// AudioTrackOption is a probed audio stream in an audio select.
type AudioTrackOption struct {
	Index int
	Label string
}

const (
	// ClipStatusPending is a queued clip that has not started encoding.
	ClipStatusPending = "pending"
	// ClipStatusProcessing is a clip that is currently encoding.
	ClipStatusProcessing = "processing"
	// ClipStatusCompleted is a clip that finished encoding.
	ClipStatusCompleted = "completed"
	// ClipStatusFailed is a clip that failed to encode.
	ClipStatusFailed = "failed"
	// ClipStatusCancelled is a clip stopped by the user.
	// The persisted job status literal is "canceled".
	ClipStatusCancelled = "canceled"
)

// ClipTypeLabel is the user-facing name for a clip type.
func ClipTypeLabel(clipType string) string {
	switch clipType {
	case "gif":
		return "GIF"
	case "screenshot":
		return "Screenshot"
	default:
		return "Video clip"
	}
}

// CanPlay reports whether a completed clip file is available for preview.
//
// Returns:
//   - playable: True when status is completed and the file exists.
func (item *ClipItem) CanPlay() bool {
	return item.Status == ClipStatusCompleted && item.FileExists
}

// DisplayName returns the clip name, or the media title when the name is empty.
//
// Returns:
//   - name: The label shown on clip cards.
func (item *ClipItem) DisplayName() string {
	if item.Name != "" {
		return item.Name
	}

	return item.MediaTitle
}

// IsActive reports whether the clip is still queued or encoding.
//
// Returns:
//   - active: True when status is pending or processing.
func (item *ClipItem) IsActive() bool {
	return item.Status == ClipStatusPending || item.Status == ClipStatusProcessing
}
