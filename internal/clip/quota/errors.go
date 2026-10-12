// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package quota

import (
	"errors"
	"strconv"
	"time"
)

// ActiveLimitError refuses a render while the member has as many waiting or
// running as they may.
type ActiveLimitError struct {
	// Limit is how many renders the member may have at once.
	Limit int
}

// WeeklyLimitError refuses a render once the member has started as many in
// seven days as they may.
type WeeklyLimitError struct {
	// Next is when the oldest counted render leaves the window.
	Next time.Time
	// Limit is how many renders the member may start in seven days.
	Limit int
}

// StorageLimitError refuses a render while the member's clips take as much
// storage as they may.
type StorageLimitError struct {
	// Limit is how many bytes the member's clips may take.
	Limit int64
}

// decimalBase is the base a byte count is written in.
const decimalBase = 10

// ErrNoLimits refuses a member's render when no limits are wired, so a
// member is never left unbounded.
var ErrNoLimits = errors.New("render limits are not available")

// Error describes the refusal.
//
// Returns:
//   - message: The limit that was reached.
func (err *ActiveLimitError) Error() string {
	return "active render limit of " + strconv.Itoa(err.Limit) + " reached"
}

// Error describes the refusal.
//
// Returns:
//   - message: The limit that was reached.
func (err *WeeklyLimitError) Error() string {
	return "weekly render limit of " + strconv.Itoa(err.Limit) + " reached"
}

// Error describes the refusal.
//
// Returns:
//   - message: The limit that was reached.
func (err *StorageLimitError) Error() string {
	return "storage limit of " + strconv.FormatInt(err.Limit, decimalBase) + " bytes reached"
}

// IsLimit reports whether an error is a render refused by a reached limit.
//
// Parameters:
//   - err: The error.
//
// Returns:
//   - reached: True for an *ActiveLimitError, *WeeklyLimitError, or
//     *StorageLimitError anywhere in the chain.
func IsLimit(err error) bool {
	//nolint:errcheck // Only the match counts, since the matched value is err itself.
	if _, active := errors.AsType[*ActiveLimitError](err); active {
		return true
	}

	//nolint:errcheck // Only the match counts, since the matched value is err itself.
	if _, weekly := errors.AsType[*WeeklyLimitError](err); weekly {
		return true
	}

	//nolint:errcheck // Only the match counts, since the matched value is err itself.
	_, storage := errors.AsType[*StorageLimitError](err)

	return storage
}
