// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"errors"
	"time"
)

// Type is the kind of export a clip produces.
type Type string

// Clip is the record a person edits: which source, which window, and which
// export. It does not carry render progress. That belongs to a Job.
type Clip struct {
	ID            string
	Type          Type
	Name          string
	MediaID       string
	MediaTitle    string
	MediaType     string
	StartTime     time.Duration
	Duration      time.Duration
	Quality       string
	Width         int
	FPS           int
	AudioIndex    int
	CropBlackBars bool
	WebSafeColor  bool
	PreserveHDR   bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const (
	// TypeClip is a video clip.
	TypeClip Type = "clip"

	// TypeGIF is an animated GIF.
	TypeGIF Type = "gif"

	// TypeScreenshot is a still frame.
	TypeScreenshot Type = "screenshot"
)

// DefaultMediaType is the media type a clip takes when none is given.
const DefaultMediaType = "movie"

// ErrUnknownType reports a requested clip type this app does not produce.
var ErrUnknownType = errors.New("clip type must be one of: clip, video, screenshot, gif")

// ParseType resolves a requested clip type.
//
// Parameters:
//   - raw: Requested type.
//
// Returns:
//   - The canonical type.
//   - False when the type is not one this app produces.
func ParseType(raw string) (Type, bool) {
	switch kind := Type(raw); kind {
	case TypeClip, TypeGIF, TypeScreenshot:
		return kind, true
	default:
		return "", false
	}
}

// ResolveType prefers a requested clip type, then the type already stored.
//
// Parameters:
//   - raw: Requested type, empty when the request named none.
//   - fallback: Type to use when the request named none.
//
// Returns:
//   - The resolved type.
//   - ErrUnknownType when raw is set but is not one this app produces.
func ResolveType(raw string, fallback Type) (Type, error) {
	if raw == "" {
		return fallback, nil
	}

	kind, ok := ParseType(raw)
	if !ok {
		return "", ErrUnknownType
	}

	return kind, nil
}

// ApplyDefaults fills the fields a clip may arrive without.
//
// Parameters:
//   - clip: Clip to fill in place.
func (clip *Clip) ApplyDefaults() {
	if clip.Type == "" {
		clip.Type = TypeClip
	}

	if clip.Quality == "" {
		clip.Quality = string(ClipQualityMedium)
	}

	if clip.MediaType == "" {
		clip.MediaType = DefaultMediaType
	}
}

// Clone returns an independent copy of the clip.
//
// Returns:
//   - copy: A clip that shares nothing with the original.
func (clip *Clip) Clone() *Clip {
	if clip == nil {
		return nil
	}

	copied := *clip

	return &copied
}
