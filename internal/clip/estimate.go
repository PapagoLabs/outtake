// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"time"
)

const (
	// minEstimatePercent is the least progress an estimate is made from,
	// because the first few percent say little about the rate.
	minEstimatePercent = 3
	// minEstimateElapsed is the least time an estimate is made from, for the
	// same reason.
	minEstimateElapsed = 5 * time.Second
	// percentDone is the progress of a finished encode.
	percentDone = 100
)

// Remaining estimates how long an encode has left.
//
// It assumes the encode keeps the rate it has had since it started. There is no
// estimate until the encode has run long enough for the rate to mean something,
// or once it is done.
//
// Parameters:
//   - started: When the encode began, zero when it has not.
//   - now: The time to estimate from.
//   - percent: How far the encode has got, from 0 to 100.
//
// Returns:
//   - left: The time left.
//   - ok: False when there is no estimate yet.
//
//nolint:nonamedreturns // The named results document what each one means.
func Remaining(started, now time.Time, percent int) (left time.Duration, ok bool) {
	if started.IsZero() || percent < minEstimatePercent || percent >= percentDone {
		return 0, false
	}

	elapsed := now.Sub(started)
	if elapsed < minEstimateElapsed {
		return 0, false
	}

	return elapsed * time.Duration(percentDone-percent) / time.Duration(percent), true
}
