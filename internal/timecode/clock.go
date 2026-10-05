// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package timecode

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// FFmpegClock is the FFmpeg time-duration clock.
type FFmpegClock struct{}

const (
	// suffixMicro is the FFmpeg microsecond duration suffix.
	suffixMicro = "us"
	// suffixMilli is the FFmpeg millisecond duration suffix.
	suffixMilli = "ms"
	// suffixSec is the FFmpeg second duration suffix.
	suffixSec = "s"
	// timecodeMinSecParts is the field count of a MM:SS[.m...] layout.
	timecodeMinSecParts = 2
	// timecodeHourMinSecParts is the field count of an HH:MM:SS[.m...] layout.
	timecodeHourMinSecParts = 3
	// signMinus is the FFmpeg duration sign prefix.
	signMinus = "-"
	// signPlus is a duration sign prefix that parsing rejects.
	signPlus = "+"
	// secondsBitSize is the bit size used when parsing decimal fields.
	secondsBitSize = 64
)

// DefaultClock is the FFmpeg duration clock.
var DefaultClock = FFmpegClock{}

// Format renders duration as HH:MM:SS.mmm.
//
// Parameters:
//   - duration: Backing duration. Negative values render as zero.
//
// Returns:
//   - text: The duration in clock form, rounded to milliseconds.
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
//
// Parameters:
//   - value: An FFmpeg time-duration string.
//
// Returns:
//   - duration: The parsed duration, negative when the value is.
//   - err: Non-nil when value is not a valid duration.
func (clock FFmpegClock) Parse(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	value, neg := strings.CutPrefix(value, signMinus)
	if value == "" || hasSignPrefix(value) {
		return 0, fmt.Errorf("%w: %s", ErrInvalidTimecode, value)
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

// parseClock parses [HH:]MM:SS[.m...], rejecting a signed, empty, or
// non-numeric field.
//
// Parameters:
//   - value: A clock-form duration.
//
// Returns:
//   - duration: The parsed duration.
//   - err: Non-nil when a field is signed, empty, or not a number.
func (FFmpegClock) parseClock(value string) (time.Duration, error) {
	parts := strings.Split(value, ":")
	count := len(parts)

	if count != timecodeMinSecParts && count != timecodeHourMinSecParts {
		return 0, ErrInvalidTimecode
	}

	if clockPartsSigned(parts) {
		return 0, ErrInvalidTimecode
	}

	sec, err := parseFiniteFloat(parts[count-1])
	if err != nil {
		return 0, fmt.Errorf("seconds: %w", err)
	}

	minutes, err := strconv.Atoi(parts[count-timecodeMinSecParts])
	if err != nil {
		return 0, fmt.Errorf("minutes: %w", err)
	}

	hours, err := clockHours(parts)
	if err != nil {
		return 0, fmt.Errorf("hours: %w", err)
	}

	converted, err := clockToDuration(hours, minutes, sec)
	if err != nil {
		return 0, fmt.Errorf("clock: %w", err)
	}

	return converted, nil
}

// parseUnsigned parses a non-negative FFmpeg duration.
//
// Parameters:
//   - value: Duration text with any sign already removed.
//
// Returns:
//   - duration: The parsed duration.
//   - err: Non-nil when value is not a valid duration.
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
//
// Parameters:
//   - field: Decimal quantity with any suffix removed.
//   - unit: The unit the quantity is counted in.
//
// Returns:
//   - duration: The converted duration.
//   - err: Non-nil when the quantity is not finite or overflows.
func parseUnit(field string, unit time.Duration) (time.Duration, error) {
	quantity, err := parseFiniteFloat(field)
	if err != nil {
		return 0, fmt.Errorf("quantity: %w", err)
	}

	converted, err := durationFromFloat(quantity, unit)
	if err != nil {
		return 0, fmt.Errorf("unit: %w", err)
	}

	return converted, nil
}

// clockToDuration converts HH, MM, SS fields to a duration.
//
// Parameters:
//   - hours: The HH field.
//   - minutes: The MM field.
//   - sec: The SS field, which may carry a fraction.
//
// Returns:
//   - duration: The summed duration.
//   - err: Non-nil when a field overflows or is not finite.
func clockToDuration(hours, minutes int, sec float64) (time.Duration, error) {
	hourDur, err := scaleDuration(hours, time.Hour)
	if err != nil {
		return 0, fmt.Errorf("hour scale: %w", err)
	}

	minDur, err := scaleDuration(minutes, time.Minute)
	if err != nil {
		return 0, fmt.Errorf("minute scale: %w", err)
	}

	secDur, err := durationFromFloat(sec, time.Second)
	if err != nil {
		return 0, fmt.Errorf("second scale: %w", err)
	}

	sum, err := addDuration(hourDur, minDur)
	if err != nil {
		return 0, fmt.Errorf("hour plus minute: %w", err)
	}

	total, err := addDuration(sum, secDur)
	if err != nil {
		return 0, fmt.Errorf("clock sum: %w", err)
	}

	return total, nil
}

// clockPartsSigned reports a signed or empty clock field.
//
// Parameters:
//   - parts: The colon-separated clock fields.
//
// Returns:
//   - invalid: True when any field is empty or carries a sign.
func clockPartsSigned(parts []string) bool {
	for _, part := range parts {
		if part == "" || hasSignPrefix(part) {
			return true
		}
	}

	return false
}

// clockHours reads the HH field, or 0 for MM:SS.
//
// Parameters:
//   - parts: The colon-separated clock fields.
//
// Returns:
//   - hours: The HH field, or 0 when the value has no hours part.
//   - err: Non-nil when the HH field is not an integer.
func clockHours(parts []string) (int, error) {
	if len(parts) != timecodeHourMinSecParts {
		return 0, nil
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("hour field: %w", err)
	}

	return hours, nil
}

// hasSignPrefix reports a leading + or - on value.
//
// Parameters:
//   - value: Field to inspect.
//
// Returns:
//   - signed: True when value starts with a sign.
func hasSignPrefix(value string) bool {
	return strings.HasPrefix(value, signMinus) || strings.HasPrefix(value, signPlus)
}

// parseFiniteFloat parses a finite decimal field.
//
// Parameters:
//   - field: Decimal text, which may be padded with whitespace.
//
// Returns:
//   - quantity: The parsed number.
//   - err: Non-nil when the field is not a number, NaN, or infinite.
func parseFiniteFloat(field string) (float64, error) {
	quantity, err := strconv.ParseFloat(strings.TrimSpace(field), secondsBitSize)
	if err != nil {
		return 0, fmt.Errorf("decimal: %w", err)
	}

	if math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return 0, ErrInvalidTimecode
	}

	return quantity, nil
}
