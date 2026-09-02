// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Clock formats and parses FFmpeg time durations.
//
// See https://ffmpeg.org/ffmpeg-utils.html#Time-duration
type Clock interface {
	Format(duration time.Duration) string
	Parse(value string) (time.Duration, error)
}

// FFmpegClock is the FFmpeg time-duration clock.
type FFmpegClock struct{}

// Timecode is a media timestamp.
type Timecode struct {
	d time.Duration
}

const (
	// SuffixMicro is the FFmpeg microsecond duration suffix.
	suffixMicro = "us"
	// SuffixMilli is the FFmpeg millisecond duration suffix.
	suffixMilli = "ms"
	// SuffixSec is the FFmpeg second duration suffix.
	suffixSec = "s"
	// TimecodeMinSecParts is MM:SS[.m...].
	timecodeMinSecParts = 2
	// TimecodeHourMinSecParts is HH:MM:SS[.m...].
	timecodeHourMinSecParts = 3
)

// ErrInvalidTimecode is returned when a timestamp string cannot be parsed.
var ErrInvalidTimecode = errors.New("invalid timecode")

// DefaultClock is the FFmpeg duration clock.
var DefaultClock Clock = FFmpegClock{}

var _ Clock = FFmpegClock{}

// FromSeconds builds a timecode from a floating-point second count.
func FromSeconds(seconds float64) Timecode {
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return Timecode{}
	}

	return Timecode{d: time.Duration(seconds * float64(time.Second))}
}

// Parse builds a timecode from an FFmpeg time-duration string.
func Parse(value string) (Timecode, error) {
	d, err := DefaultClock.Parse(value)
	if err != nil {
		return Timecode{}, fmt.Errorf("parse timecode: %w", err)
	}

	return Timecode{d: d}, nil
}

// Duration returns the backing duration.
func (t Timecode) Duration() time.Duration {
	return t.d
}

// HMS renders HH:MM:SS, rounding to the nearest second.
func (t Timecode) HMS() string {
	duration := max(t.d.Round(time.Second), 0)
	hours := duration / time.Hour
	minutes := (duration % time.Hour) / time.Minute
	seconds := (duration % time.Minute) / time.Second

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// Seconds returns the timestamp in seconds.
func (t Timecode) Seconds() float64 {
	return t.d.Seconds()
}

// String renders HH:MM:SS.mmm.
func (t Timecode) String() string {
	return DefaultClock.Format(t.d)
}

// Format renders duration as HH:MM:SS.mmm.
func (FFmpegClock) Format(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}

	duration = duration.Round(time.Millisecond)

	hours := duration / time.Hour
	minutes := (duration % time.Hour) / time.Minute
	seconds := (duration % time.Minute) / time.Second
	millis := (duration % time.Second) / time.Millisecond

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

// Parse accepts FFmpeg time durations: [-][HH:]MM:SS[.m...], [-]S+[.m...][s|ms|us].
func (clock FFmpegClock) Parse(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	neg := strings.HasPrefix(value, "-")
	if neg {
		value = strings.TrimPrefix(value, "-")
	}

	duration, err := clock.parseUnsigned(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidTimecode, value)
	}

	if neg {
		duration = -duration
	}

	return duration, nil
}

// parseClock parses [HH:]MM:SS[.m...].
func (FFmpegClock) parseClock(value string) (time.Duration, error) {
	parts := strings.Split(value, ":")
	count := len(parts)

	if count != timecodeMinSecParts && count != timecodeHourMinSecParts {
		return 0, ErrInvalidTimecode
	}

	sec, err := strconv.ParseFloat(parts[count-1], secondsBitSize)
	if err != nil {
		return 0, fmt.Errorf("seconds: %w", err)
	}

	minutes, err := strconv.Atoi(parts[count-timecodeMinSecParts])
	if err != nil {
		return 0, fmt.Errorf("minutes: %w", err)
	}

	hours := 0
	if count == timecodeHourMinSecParts {
		hours, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("hours: %w", err)
		}
	}

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(sec*float64(time.Second)), nil
}

// parseUnsigned parses a non-negative FFmpeg duration.
func (clock FFmpegClock) parseUnsigned(value string) (time.Duration, error) {
	var (
		duration time.Duration
		err      error
	)

	switch {
	case strings.HasSuffix(value, suffixMicro):
		duration, err = parseUnit(strings.TrimSuffix(value, suffixMicro), time.Microsecond)
	case strings.HasSuffix(value, suffixMilli):
		duration, err = parseUnit(strings.TrimSuffix(value, suffixMilli), time.Millisecond)
	case strings.Contains(value, ":"):
		duration, err = clock.parseClock(value)
	default:
		duration, err = parseUnit(strings.TrimSuffix(value, suffixSec), time.Second)
	}

	if err != nil {
		return 0, fmt.Errorf("unsigned duration: %w", err)
	}

	return duration, nil
}

// parseUnit parses a numeric duration in the given unit.
func parseUnit(field string, unit time.Duration) (time.Duration, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(field), secondsBitSize)
	if err != nil {
		return 0, fmt.Errorf("quantity: %w", err)
	}

	return time.Duration(n * float64(unit)), nil
}
