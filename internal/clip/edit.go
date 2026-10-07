// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"fmt"
	"time"
)

// Edit is the change a clip update applies to a stored clip.
type Edit struct {
	// Type is the type the clip becomes, empty to keep the stored one.
	Type Type
	// Name is the clip name, empty to keep the stored one.
	Name string
	// Quality is the encode profile id, empty to keep the stored one.
	Quality string
	// Start is where the clip begins in the source.
	Start time.Duration
	// Length is how much of the source the clip covers.
	Length time.Duration
	// Width is the GIF export width, which other types ignore.
	Width int
	// FPS is the GIF frame rate, which other types ignore.
	FPS int
	// AudioIndex is the audio stream the clip renders.
	AudioIndex int
	// CropBlackBars reports whether black bars are trimmed.
	CropBlackBars bool
	// WebSafeColor is the tone-map setting, nil when the edit carried none.
	WebSafeColor *bool
	// PreserveHDR keeps the source HDR transfer, nil when the edit carried none.
	PreserveHDR *bool
}

// EditRequest is a clip update as a caller sends it. A field left out keeps
// the value the clip already has, so a caller may send only what changes.
type EditRequest struct {
	Name          *string  `json:"name"`
	ClipType      *string  `json:"clipType"`
	StartTime     *float64 `json:"startTime"`
	Duration      *float64 `json:"duration"`
	Quality       *string  `json:"quality"`
	Width         *int     `json:"width"`
	FPS           *int     `json:"fps"`
	AudioIndex    *int     `json:"audioIndex"`
	CropBlackBars *bool    `json:"cropBlackBars"`
	WebSafeColor  *bool    `json:"webSafeColor"`
	PreserveHDR   *bool    `json:"preserveHdr"`
}

// GIF encoder bounds, which only a GIF is held to.
const (
	// MinGIFWidth is the lowest GIF export width accepted.
	MinGIFWidth = 120
	// MaxGIFWidth is the highest GIF export width accepted.
	MaxGIFWidth = 1920
	// MinGIFFPS is the lowest GIF frame rate accepted.
	MinGIFFPS = 5
	// MaxGIFFPS is the highest GIF frame rate accepted.
	MaxGIFFPS = 30
)

var (
	// ErrInvalidGIFWidth reports a GIF width outside the encoder bounds.
	ErrInvalidGIFWidth = fmt.Errorf(
		"gif width must be between %d and %d",
		MinGIFWidth,
		MaxGIFWidth,
	)

	// ErrInvalidGIFFPS reports a GIF frame rate outside the encoder bounds.
	ErrInvalidGIFFPS = fmt.Errorf(
		"gif fps must be between %d and %d",
		MinGIFFPS,
		MaxGIFFPS,
	)
)

// Validate reports whether the edit may be stored.
//
// Parameters:
//   - kind: Normalized clip type.
//   - sourceLength: How long the source runs, zero when it could not be probed.
//   - limit: Longest clip the installation accepts.
//
// Returns:
//   - err: Non-nil when a bound is violated.
func (edit Edit) Validate(kind Type, sourceLength, limit time.Duration) error {
	selection := Selection{
		Start:        edit.Start,
		Length:       edit.Length,
		SourceLength: sourceLength,
	}

	err := selection.Validate(kind, limit)
	if err != nil {
		return fmt.Errorf("validate duration: %w", err)
	}

	err = selection.WithinSource(kind)
	if err != nil {
		return fmt.Errorf("check range: %w", err)
	}

	err = validateGIF(kind, edit.Width, edit.FPS)
	if err != nil {
		return fmt.Errorf("validate gif: %w", err)
	}

	return nil
}

// Apply writes the editable fields of an edit onto a stored clip.
//
// Parameters:
//   - edit: The change to apply.
func (clip *Clip) Apply(edit Edit) {
	if edit.Type != "" {
		clip.Type = edit.Type
	}

	if edit.Name != "" {
		clip.Name = edit.Name
	}

	if edit.Quality != "" {
		clip.Quality = edit.Quality
	}

	clip.StartTime = edit.Start
	clip.Duration = edit.Length
	clip.Width = edit.Width
	clip.FPS = edit.FPS
	clip.AudioIndex = edit.AudioIndex
	clip.CropBlackBars = edit.CropBlackBars

	if edit.WebSafeColor != nil {
		clip.WebSafeColor = *edit.WebSafeColor
	}

	if edit.PreserveHDR != nil {
		clip.PreserveHDR = *edit.PreserveHDR
	}

	clip.UpdatedAt = time.Now()
}

// RendersLike reports whether two clips produce the same file, so that a
// change between them is only to how the clip is named or listed.
//
// Parameters:
//   - other: The clip to compare against.
//
// Returns:
//   - same: True when every field the render reads matches.
func (clip *Clip) RendersLike(other *Clip) bool {
	return clip.selectsLike(other) && clip.encodesLike(other)
}

// encodesLike reports whether two clips encode their frames the same way.
//
// Parameters:
//   - other: The clip to compare against.
//
// Returns:
//   - same: True when the profile, size, rate, audio, and color match.
func (clip *Clip) encodesLike(other *Clip) bool {
	return clip.Quality == other.Quality &&
		clip.Width == other.Width &&
		clip.FPS == other.FPS &&
		clip.AudioIndex == other.AudioIndex &&
		clip.CropBlackBars == other.CropBlackBars &&
		clip.WebSafeColor == other.WebSafeColor &&
		clip.PreserveHDR == other.PreserveHDR
}

// selectsLike reports whether two clips cut the same part of the same source
// into the same kind of file.
//
// Parameters:
//   - other: The clip to compare against.
//
// Returns:
//   - same: True when the type, source, and window match.
func (clip *Clip) selectsLike(other *Clip) bool {
	return clip.Type == other.Type &&
		clip.MediaID == other.MediaID &&
		clip.StartTime == other.StartTime &&
		clip.Duration == other.Duration
}

// validateGIF enforces the GIF width and frame rate bounds.
//
// Parameters:
//   - kind: Normalized clip type.
//   - width: Requested GIF width in pixels.
//   - fps: Requested GIF frame rate.
//
// Returns:
//   - err: Non-nil when the clip is not a GIF or a field is out of bounds.
func validateGIF(kind Type, width, fps int) error {
	if kind != TypeGIF {
		return nil
	}

	if width != 0 && (width < MinGIFWidth || width > MaxGIFWidth) {
		return ErrInvalidGIFWidth
	}

	if fps != 0 && (fps < MinGIFFPS || fps > MaxGIFFPS) {
		return ErrInvalidGIFFPS
	}

	return nil
}
