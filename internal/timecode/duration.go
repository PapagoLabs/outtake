// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package timecode

import (
	"math"
	"time"
)

// durationFromFloat converts quantity*unit to a Duration, rejecting overflow.
//
// Parameters:
//   - quantity: Non-negative multiplier.
//   - unit: The unit the quantity is counted in.
//
// Returns:
//   - duration: The converted duration.
//   - err: ErrInvalidTimecode when quantity is negative, not finite, or the
//     product overflows.
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

// scaleDuration converts count*unit to a Duration, rejecting overflow.
//
// Parameters:
//   - count: Non-negative multiplier.
//   - unit: The unit the count is counted in.
//
// Returns:
//   - duration: The converted duration.
//   - err: ErrInvalidTimecode when count is negative or the product overflows.
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
//   - left: The first duration.
//   - right: The second duration.
//
// Returns:
//   - duration: The sum.
//   - err: ErrInvalidTimecode when the sum overflows.
func addDuration(left, right time.Duration) (time.Duration, error) {
	if right > 0 && left > time.Duration(math.MaxInt64)-right {
		return 0, ErrInvalidTimecode
	}

	return left + right, nil
}
