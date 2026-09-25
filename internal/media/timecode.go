// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"fmt"
	"strconv"
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

const (
	// TimecodeDecimals is the number of fractional second digits used when a
	// timecode crosses a boundary that only carries seconds, such as a query
	// parameter or a button offset. It matches the millisecond precision that
	// Clock.Parse accepts.
	TimecodeDecimals = 3
	// TimecodeBitSize is the float bit size used to format seconds.
	TimecodeBitSize = 64
)

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
//   - parsed: The parsed timestamp.
//   - err: Non-nil when value is not a valid duration.
func Parse(value string) (Timecode, error) {
	d, err := DefaultClock.Parse(value)
	if err != nil {
		return Timecode{}, fmt.Errorf("parse: %w", err)
	}

	return timecode.FromDuration(d), nil
}

// FormatSeconds renders seconds as a fixed-point decimal with millisecond
// precision.
//
// Start and end marks are handed to the browser through query parameters and
// read back with [strconv.ParseFloat], so this value has to survive a round
// trip. Truncating below millisecond precision moves the user's marks on every
// round trip, which is invisible on a seconds-only view and destructive when
// the edit is frame-accurate.
//
// Parameters:
//   - seconds: Timestamp in seconds.
//
// Returns:
//   - text: A decimal string carrying exactly TimecodeDecimals fractional digits.
func FormatSeconds(seconds float64) string {
	return strconv.FormatFloat(seconds, 'f', TimecodeDecimals, TimecodeBitSize)
}
