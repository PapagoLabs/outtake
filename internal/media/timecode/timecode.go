// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package timecode

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// FFmpegClock is the FFmpeg time-duration clock.
type FFmpegClock struct{}

// Timecode is a media timestamp.
type Timecode struct {
	d time.Duration
}

const (
	// suffixMicro is the FFmpeg microsecond duration suffix.
	suffixMicro = "us"
	// suffixMilli is the FFmpeg millisecond duration suffix.
	suffixMilli = "ms"
	// suffixSec is the FFmpeg second duration suffix.
	suffixSec = "s"
	// timecodeMinSecParts is MM:SS[.m...].
	timecodeMinSecParts = 2
	// timecodeHourMinSecParts is HH:MM:SS[.m...].
	timecodeHourMinSecParts = 3
	// signMinus is the FFmpeg duration sign prefix.
	signMinus = "-"
	// signPlus is a rejected duration sign prefix.
	signPlus = "+"
	// secondsBitSize is the bit size used when parsing decimal fields.
	secondsBitSize = 64
)

// ErrInvalidTimecode is returned when a timestamp string cannot be parsed.
var ErrInvalidTimecode = errors.New("invalid timecode")

// DefaultClock is the FFmpeg duration clock.
var DefaultClock = FFmpegClock{}

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

// HMS renders HH:MM:SS, rounding to the nearest second.
//
// Returns:
//   - clock: The timestamp as HH:MM:SS.
func (t Timecode) HMS() string {
	duration := max(t.d.Round(time.Second), 0)
	hours := duration / time.Hour
	minutes := (duration % time.Hour) / time.Minute
	seconds := (duration % time.Minute) / time.Second

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// Seconds returns the timestamp in seconds.
//
// Returns:
//   - seconds: The timestamp as a floating-point second count.
func (t Timecode) Seconds() float64 {
	return t.d.Seconds()
}

// String renders HH:MM:SS.mmm.
//
// Returns:
//   - value: Result value. Zero or empty when unavailable.
func (t Timecode) String() string {
	return DefaultClock.Format(t.d)
}

// Format renders duration as HH:MM:SS.mmm.
//
// Parameters:
//   - duration: Timeout or interval duration.
//
// Returns:
//   - value: Result value. Zero or empty when unavailable.
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

// Parse accepts FFmpeg time durations: [-][HH:]MM:SS[.m...],
// [-]S+[.m...][s|ms|us].
//
// Parameters:
//   - value: String value to convert or validate.
//
// Returns:
//   - dur: Result of Parse.
//   - err: Wrapped failure from "...: ...".
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

// parseClock parses [HH:]MM:SS[.m...].
//
// Parameters:
//   - value: String value to convert or validate.
//
// Returns:
//   - dur: The [HH:]MM:SS[.m...].
//   - err: Wrapped failure such as "seconds", "minutes", or "hours".
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

// clockToDuration converts HH, MM, SS fields to a duration.
//
// Parameters:
//   - hours: Typed int argument for clockToDuration.
//   - minutes: Typed int argument for clockToDuration.
//   - sec: Typed float64 argument for clockToDuration.
//
// Returns:
//   - dur: Result of clockToDuration.
//   - err: Wrapped failure such as "hour scale", "minute scale", or "second
//     scale".
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
//   - parts: Typed []string argument for clockPartsSigned.
//
// Returns:
//   - ok: True when the condition holds.
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
//   - parts: Typed []string argument for clockHours.
//
// Returns:
//   - n: The HH field, or 0 for MM:SS.
//   - err: Wrapped failure from "hour field".
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
//   - value: String value to convert or validate.
//
// Returns:
//   - ok: True when the condition holds.
func hasSignPrefix(value string) bool {
	return strings.HasPrefix(value, signMinus) || strings.HasPrefix(value, signPlus)
}

// parseUnsigned parses a non-negative FFmpeg duration.
//
// Parameters:
//   - value: String value to convert or validate.
//
// Returns:
//   - dur: A non-negative FFmpeg duration.
//   - err: Wrapped failure from "unsigned duration".
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
//   - field: Typed string argument for parseUnit.
//   - unit: Typed time.Duration argument for parseUnit.
//
// Returns:
//   - dur: A numeric duration in the given unit.
//   - err: Wrapped failure such as "quantity" or "unit".
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

// durationFromFloat converts quantity*unit to a Duration, rejecting overflow.
//
// Parameters:
//   - quantity: Typed float64 argument for durationFromFloat.
//   - unit: Typed time.Duration argument for durationFromFloat.
//
// Returns:
//   - dur: Result of durationFromFloat.
//   - err: Failure from decimal.
func durationFromFloat(quantity float64, unit time.Duration) (time.Duration, error) {
	if quantity < 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) || unit <= 0 {
		return 0, ErrInvalidTimecode
	}

	maxQuantity := float64(math.MaxInt64) / float64(unit)
	if quantity >= maxQuantity {
		return 0, ErrInvalidTimecode
	}

	converted := time.Duration(quantity * float64(unit))
	if converted < 0 {
		return 0, ErrInvalidTimecode
	}

	return converted, nil
}

// scaleDuration converts n*unit to a Duration, rejecting overflow.
//
// Parameters:
//   - count: Typed int argument for scaleDuration.
//   - unit: Typed time.Duration argument for scaleDuration.
//
// Returns:
//   - dur: Result of scaleDuration.
//   - err: Failure from decimal.
func scaleDuration(count int, unit time.Duration) (time.Duration, error) {
	if count < 0 || unit <= 0 {
		return 0, ErrInvalidTimecode
	}

	if count != 0 && int64(unit) > math.MaxInt64/int64(count) {
		return 0, ErrInvalidTimecode
	}

	return time.Duration(count) * unit, nil
}

// addDuration adds two durations, rejecting overflow.
//
// Parameters:
//   - left: Left operand for comparison.
//   - right: Right operand for comparison.
//
// Returns:
//   - dur: Result of addDuration.
//   - err: Failure from decimal.
func addDuration(left, right time.Duration) (time.Duration, error) {
	if right > 0 && left > time.Duration(math.MaxInt64)-right {
		return 0, ErrInvalidTimecode
	}

	return left + right, nil
}

// parseFiniteFloat parses a finite decimal field.
//
// Parameters:
//   - field: Typed string argument for parseFiniteFloat.
//
// Returns:
//   - value: A finite decimal field.
//   - err: Failure from decimal.
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
