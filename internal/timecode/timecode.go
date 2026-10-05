// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package timecode

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Timecode is a media timestamp.
type Timecode struct {
	d time.Duration
}

const (
	// decimalBase is the base used when formatting a number as text.
	decimalBase = 10
	// shortParts is how many units a short duration can carry.
	shortParts = 3
)

// ErrInvalidTimecode is returned when a timestamp string cannot be parsed.
var ErrInvalidTimecode = errors.New("invalid timecode")

// FromSeconds builds a timecode from a floating-point second count.
//
// Parameters:
//   - seconds: Timestamp in seconds. Negative, NaN, and Inf become zero.
//
// Returns:
//   - timecode: The timestamp, or a zero value when seconds is not finite.
func FromSeconds(seconds float64) Timecode {
	d, err := durationFromFloat(seconds, time.Second)
	if err != nil {
		return Timecode{}
	}

	return Timecode{d: d}
}

// FromDuration builds a timecode from a duration.
//
// Parameters:
//   - d: Backing duration. Signed values are kept.
//
// Returns:
//   - timecode: The timestamp.
func FromDuration(d time.Duration) Timecode {
	return Timecode{d: d}
}

// Parse builds a timecode from an FFmpeg time-duration string.
//
// Parameters:
//   - value: An FFmpeg time-duration string.
//
// Returns:
//   - timecode: The parsed timestamp.
//   - err: Non-nil when value is not a valid duration.
func Parse(value string) (Timecode, error) {
	d, err := DefaultClock.Parse(value)
	if err != nil {
		return Timecode{}, fmt.Errorf("parse timecode: %w", err)
	}

	return Timecode{d: d}, nil
}

// Duration returns the backing duration.
//
// Returns:
//   - duration: The stored [time.Duration].
func (t Timecode) Duration() time.Duration {
	return t.d
}

// FormatSeconds renders the timestamp as a fixed-point decimal carrying exactly
// three fractional digits. The value is rounded to the nearest millisecond,
// matching [FFmpegClock.Format], and negative values render as zero.
//
// Returns:
//   - text: A decimal string carrying exactly three fractional digits.
func (t Timecode) FormatSeconds() string {
	const millisPerSecond = int64(time.Second / time.Millisecond)

	millis := max(t.d.Round(time.Millisecond), 0).Milliseconds()

	return fmt.Sprintf("%d.%03d", millis/millisPerSecond, millis%millisPerSecond)
}

// Short renders a duration in the short form, with empty parts left out.
//
// Returns:
//   - short: The duration in short form.
func (t Timecode) Short() string {
	duration := max(t.d.Round(time.Second), 0)
	if duration < time.Minute {
		return strconv.FormatInt(int64(duration/time.Second), decimalBase) + "s"
	}

	parts := make([]string, 0, shortParts)

	if hours := int64(duration / time.Hour); hours > 0 {
		parts = append(parts, strconv.FormatInt(hours, decimalBase)+"hr")
	}

	if minutes := int64((duration % time.Hour) / time.Minute); minutes > 0 {
		parts = append(parts, strconv.FormatInt(minutes, decimalBase)+"min")
	}

	if seconds := int64((duration % time.Minute) / time.Second); seconds > 0 {
		parts = append(parts, strconv.FormatInt(seconds, decimalBase)+"s")
	}

	return strings.Join(parts, "")
}

// String renders HH:MM:SS.mmm.
//
// Returns:
//   - text: The timestamp in clock form.
func (t Timecode) String() string {
	return DefaultClock.Format(t.d)
}

// FormatSeconds renders seconds as a fixed-point decimal carrying exactly three
// fractional digits. Negative or non-finite values render as zero.
//
// Parameters:
//   - seconds: Timestamp in seconds.
//
// Returns:
//   - text: A decimal string carrying exactly three fractional digits.
func FormatSeconds(seconds float64) string {
	return FromSeconds(seconds).FormatSeconds()
}
