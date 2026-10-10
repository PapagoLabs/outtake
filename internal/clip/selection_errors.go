// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/timecode"
)

// SelectionTooLongError reports a selection longer than a clip may be. It
// carries the limit, so the page can name it.
type SelectionTooLongError struct {
	// Limit is the longest a clip may be.
	Limit time.Duration
}

// PastSourceEndError reports a mark past the end of the source. It carries
// the source's length, so the page can name it.
type PastSourceEndError struct {
	// Edge names the mark, MarkStart or MarkEnd.
	Edge string
	// Mark is where the selection asked the mark to be.
	Mark time.Duration
	// SourceLength is how long the source runs.
	SourceLength time.Duration
}

// MissingAudioTrackError reports an audio track the source does not carry.
// It carries the track the user chose, counted from one.
type MissingAudioTrackError struct {
	// Track is the chosen track, counted from one.
	Track int
	// Count is how many audio tracks the source carries.
	Count int
}

const (
	// MarkStart names a selection's start mark.
	MarkStart = "start"
	// MarkEnd names a selection's end mark.
	MarkEnd = "end"
)

var (
	// ErrEmptyRange reports a clip or GIF whose end is not after its start.
	ErrEmptyRange = fmt.Errorf("%w: the range must be longer than zero", ErrInvalidDuration)

	// ErrNegativeStart reports a selection that starts before the source.
	ErrNegativeStart = fmt.Errorf("%w: the start must not be negative", ErrRangeOutsideMedia)

	// ErrNegativeLength reports a selection whose length is below zero.
	ErrNegativeLength = fmt.Errorf("%w: the length must not be negative", ErrInvalidDuration)

	// ErrNegativeAudioTrack reports an audio track index below zero.
	ErrNegativeAudioTrack = fmt.Errorf("%w: the track must not be negative", ErrNoSuchAudioTrack)
)

// Error describes the selection that is too long.
//
// Returns:
//   - message: The error text.
func (err *SelectionTooLongError) Error() string {
	return fmt.Sprintf(
		"%v: must be between 0 and %v seconds",
		ErrInvalidDuration,
		err.Limit.Seconds(),
	)
}

// Unwrap ties the error to ErrInvalidDuration.
//
// Returns:
//   - target: ErrInvalidDuration.
func (*SelectionTooLongError) Unwrap() error {
	return ErrInvalidDuration
}

// Error describes the mark past the end of the source.
//
// Returns:
//   - message: The error text.
func (err *PastSourceEndError) Error() string {
	return fmt.Sprintf(
		"%v: the %s is %s but the media is only %s long",
		ErrRangeOutsideMedia,
		err.Edge,
		timecode.FromDuration(err.Mark).Short(),
		timecode.FromDuration(err.SourceLength).Short(),
	)
}

// Unwrap ties the error to ErrRangeOutsideMedia.
//
// Returns:
//   - target: ErrRangeOutsideMedia.
func (*PastSourceEndError) Unwrap() error {
	return ErrRangeOutsideMedia
}

// Error describes the missing audio track.
//
// Returns:
//   - message: The error text.
func (err *MissingAudioTrackError) Error() string {
	return fmt.Sprintf(
		"%v: track %d was asked for, but the source carries %d",
		ErrNoSuchAudioTrack,
		err.Track,
		err.Count,
	)
}

// Unwrap ties the error to ErrNoSuchAudioTrack.
//
// Returns:
//   - target: ErrNoSuchAudioTrack.
func (*MissingAudioTrackError) Unwrap() error {
	return ErrNoSuchAudioTrack
}
