// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package quota

import (
	"time"
)

// Limits bounds what one member may render.
//
// A zero field sets no limit. A zero ClipLength leaves the installation's own
// maximum clip length as the only cap.
type Limits struct {
	// ClipLength is the longest clip the member may make.
	ClipLength time.Duration
	// Storage is how many bytes the member's clips may take.
	Storage int64
	// Active is how many of the member's renders may wait or run at once.
	Active int
	// Weekly is how many renders the member may start in any seven days.
	Weekly int
}

// Override is one member's own limits.
//
// A nil field takes the global value, and a zero one sets no limit.
type Override struct {
	// ClipLength replaces the global longest clip.
	ClipLength *time.Duration
	// Storage replaces the global storage limit.
	Storage *int64
	// Active replaces the global limit on concurrent renders.
	Active *int
	// Weekly replaces the global weekly limit.
	Weekly *int
}

// Default limits for every member until an admin sets others.
const (
	// DefaultActive is how many renders a member may have at once.
	DefaultActive = 2
	// DefaultWeekly is how many renders a member may start in seven days.
	DefaultWeekly = 100
	// DefaultStorage is how many bytes a member's clips may take, 25 GB.
	DefaultStorage int64 = 25_000_000_000
)

// Window is the span the weekly limit counts renders over.
const Window = 7 * 24 * time.Hour

// Defaults returns the limits every member has until an admin sets others.
//
// Returns:
//   - limits: Two concurrent renders, 100 a week, 25 GB, and clips up to the
//     installation's maximum length.
func Defaults() Limits {
	return Limits{
		ClipLength: 0,
		Storage:    DefaultStorage,
		Active:     DefaultActive,
		Weekly:     DefaultWeekly,
	}
}

// Apply lays a member's own limits over the global ones.
//
// Parameters:
//   - global: The limits every member has.
//
// Returns:
//   - limits: The member's limits.
func (override Override) Apply(global Limits) Limits {
	limits := global

	if override.ClipLength != nil {
		limits.ClipLength = *override.ClipLength
	}

	if override.Storage != nil {
		limits.Storage = *override.Storage
	}

	if override.Active != nil {
		limits.Active = *override.Active
	}

	if override.Weekly != nil {
		limits.Weekly = *override.Weekly
	}

	return limits
}

// ClipCap returns the longest clip a member may make.
//
// Parameters:
//   - installation: The installation's own maximum clip length.
//
// Returns:
//   - limit: The shorter of the member's limit and the installation's
//     maximum, or the installation's maximum when the member has none.
func (limits Limits) ClipCap(installation time.Duration) time.Duration {
	if limits.ClipLength <= 0 || (installation > 0 && installation < limits.ClipLength) {
		return installation
	}

	return limits.ClipLength
}
