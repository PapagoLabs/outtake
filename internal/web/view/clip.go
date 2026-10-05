// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
)

// ClipItem is one clip card on the clips list and media item pages.
type ClipItem struct {
	ID            string
	Name          string
	MediaID       string
	MediaTitle    string
	ClipType      clip.Type
	Status        clip.Status
	Progress      int
	CreatedAt     string
	StartTime     time.Duration
	Duration      time.Duration
	Quality       string
	ProfileName   string
	Profiles      []profile.ProfileOption
	FileExists    bool
	AudioIndex    int
	AudioTracks   []AudioTrackOption
	CropBlackBars bool
	WebSafeColor  bool
	PreserveHDR   bool
	SourceHDR     bool
	MediaDuration time.Duration
	Width         int
	FPS           int
	MaxDur        time.Duration
	Error         string
}

// AudioTrackOption is a probed audio stream in an audio select.
type AudioTrackOption struct {
	Index int
	Label string
}

// clipTimeLayout is how a clip timestamp is rendered.
const clipTimeLayout = "Jan 2, 2006 3:04 PM"

const (
	// ClipSortCreatedDesc lists newest created clips first.
	ClipSortCreatedDesc = catalog.SortCreatedDesc
	// ClipSortCreatedAsc lists oldest created clips first.
	ClipSortCreatedAsc = catalog.SortCreatedAsc
	// ClipSortUpdatedDesc lists recently modified clips first.
	ClipSortUpdatedDesc = catalog.SortUpdatedDesc
	// ClipSortUpdatedAsc lists oldest modified clips first.
	ClipSortUpdatedAsc = catalog.SortUpdatedAsc
	// ClipSortNameAsc lists clips by display name A-Z.
	ClipSortNameAsc = catalog.SortNameAsc
	// ClipSortNameDesc lists clips by display name Z-A.
	ClipSortNameDesc = catalog.SortNameDesc
)

const (
	// DefaultGIFWidth is the GIF export width used when a clip carries none.
	DefaultGIFWidth = 480
	// DefaultGIFFPS is the GIF export frame rate used when a clip carries none.
	DefaultGIFFPS = 10
)

// GIFWidthOrDefault returns the GIF export width to show.
//
// Parameters:
//   - width: Carried GIF width, where zero means the field was never set.
//
// Returns:
//   - value: width when it was carried, otherwise DefaultGIFWidth.
func GIFWidthOrDefault(width int) int {
	if width > 0 {
		return width
	}

	return DefaultGIFWidth
}

// GIFFPSOrDefault returns the GIF export frame rate to show.
//
// Parameters:
//   - fps: Carried GIF frame rate, where zero means the field was never set.
//
// Returns:
//   - value: fps when it was carried, otherwise DefaultGIFFPS.
func GIFFPSOrDefault(fps int) int {
	if fps > 0 {
		return fps
	}

	return DefaultGIFFPS
}

// ClipTypeLabel is the user-facing name for a clip type.
//
// Parameters:
//   - clipType: Clip type recorded for the clip.
//
// Returns:
//   - label: The display name, or Clip for a type that is neither kind.
func ClipTypeLabel(clipType clip.Type) string {
	switch clipType {
	case clip.TypeGIF:
		return "GIF"
	case clip.TypeScreenshot:
		return "Screenshot"
	default:
		return "Clip"
	}
}

// CanPlay reports whether a completed clip file is available for preview.
//
// Returns:
//   - playable: True when status is completed and the file exists.
func (item *ClipItem) CanPlay() bool {
	return item.Status == clip.StatusCompleted && item.FileExists
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

// EndTime is the clip end as start plus duration.
//
// Returns:
//   - end: StartTime + Duration.
func (item *ClipItem) EndTime() time.Duration {
	return item.StartTime + item.Duration
}

// GIFFPS is the GIF frame rate, or the New export default when unset.
//
// Returns:
//   - fps: Stored fps, or 10 when FPS is 0.
func (item *ClipItem) GIFFPS() int {
	return GIFFPSOrDefault(item.FPS)
}

// GIFWidth is the GIF export width, or the New export default when unset.
//
// Returns:
//   - width: Stored width, or 480 when Width is 0.
func (item *ClipItem) GIFWidth() int {
	return GIFWidthOrDefault(item.Width)
}

// IsActive reports whether the clip is still queued or encoding.
//
// Returns:
//   - active: True when status is pending or processing.
func (item *ClipItem) IsActive() bool {
	return item.Status == clip.StatusPending || item.Status == clip.StatusProcessing
}

// FormatClipCreated renders a clip timestamp for display, or blank when unset.
//
// Parameters:
//   - created: Clip creation time.
//
// Returns:
//   - stamp: Formatted UTC timestamp, or an empty string for a zero time.
func FormatClipCreated(created time.Time) string {
	if created.IsZero() {
		return ""
	}

	return created.UTC().Format(clipTimeLayout)
}
