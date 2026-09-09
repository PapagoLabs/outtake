// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/media/timecode"
)

// Clock formats and parses FFmpeg time durations.
//
// See https://ffmpeg.org/ffmpeg-utils.html#Time-duration
type Clock interface {
	Format(duration time.Duration) string
	Parse(value string) (time.Duration, error)
}

// FFmpegClock is the FFmpeg time-duration clock.
type FFmpegClock = timecode.FFmpegClock

// Timecode is a media timestamp.
type Timecode = timecode.Timecode

// ErrInvalidTimecode is returned when a timestamp string cannot be parsed.
var ErrInvalidTimecode = timecode.ErrInvalidTimecode

// DefaultClock is the FFmpeg duration clock.
var DefaultClock Clock = timecode.DefaultClock

var _ Clock = FFmpegClock{}

// FromSeconds builds a timecode from a floating-point second count.
//
// Parameters:
//   - seconds: Timestamp in seconds. Negative, NaN, Inf, and values outside
//     the [time.Duration] range become zero.
//
// Returns:
//   - timecode: The timestamp, or a zero value when seconds is not finite.
func FromSeconds(seconds float64) Timecode {
	return timecode.FromSeconds(seconds)
}

// Parse builds a timecode from an FFmpeg time-duration string.
//
// Parameters:
//   - value: An FFmpeg time-duration string.
//
// Returns:
//   - parsed: Result of Parse.
//   - err: Non-nil when value is not a valid duration.
func Parse(value string) (Timecode, error) {
	d, err := DefaultClock.Parse(value)
	if err != nil {
		return Timecode{}, fmt.Errorf("parse: %w", err)
	}

	return timecode.FromDuration(d), nil
}
