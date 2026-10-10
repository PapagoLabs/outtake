// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"errors"
	"fmt"
	"time"
)

// Selection is the window of a source a clip renders.
type Selection struct {
	// Start is where the selection begins in the source.
	Start time.Duration
	// Length is how much of the source the selection covers, zero for a screenshot.
	Length time.Duration
	// SourceLength is how long the source runs, zero when it could not be probed.
	SourceLength time.Duration
}

var (
	// ErrInvalidDuration reports a clip duration out of range.
	ErrInvalidDuration = errors.New("invalid duration")

	// ErrRangeOutsideMedia reports a selection reaching past the end of the source.
	ErrRangeOutsideMedia = errors.New("selection is outside the media")
)

// BoundedLength is the span a type is actually bounded by, which for a
// screenshot is the single frame it renders rather than a range.
//
// Parameters:
//   - kind: Normalized clip type.
//   - length: Selection length as submitted.
//
// Returns:
//   - bounded: The length to bound by.
func BoundedLength(kind Type, length time.Duration) time.Duration {
	if kind == TypeScreenshot {
		return 0
	}

	return length
}

// Validate reports whether the length a type renders is within the cap.
//
// Parameters:
//   - kind: Normalized clip type.
//   - limit: Longest clip the installation accepts.
//
// Returns:
//   - err: Non-nil when the length is negative or outside the cap.
func (selection Selection) Validate(kind Type, limit time.Duration) error {
	if kind == TypeScreenshot {
		if selection.Length < 0 {
			return fmt.Errorf("%w: must be zero or greater", ErrInvalidDuration)
		}

		return nil
	}

	if selection.Length <= 0 {
		return ErrEmptyRange
	}

	if selection.Length > limit {
		return &SelectionTooLongError{Limit: limit}
	}

	return nil
}

// WithinSource reports whether the selection fits inside the source.
//
// Parameters:
//   - kind: Normalized clip type.
//
// Returns:
//   - err: Non-nil when the selection reaches past the end of the source.
func (selection Selection) WithinSource(kind Type) error {
	if selection.Start < 0 {
		return ErrNegativeStart
	}

	if selection.SourceLength <= 0 {
		return nil
	}

	if selection.Start >= selection.SourceLength {
		return &PastSourceEndError{
			Edge:         MarkStart,
			Mark:         selection.Start,
			SourceLength: selection.SourceLength,
		}
	}

	end := selection.Start + BoundedLength(kind, selection.Length)
	if end > selection.SourceLength {
		return &PastSourceEndError{Edge: MarkEnd, Mark: end, SourceLength: selection.SourceLength}
	}

	return nil
}
