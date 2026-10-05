// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"time"
)

// MaxDuration is the longest clip accepted when no cap is configured.
const MaxDuration = 10 * time.Minute

// DurationCap resolves a configured cap against the default.
//
// Parameters:
//   - configured: Cap read from configuration.
//
// Returns:
//   - cap: The effective maximum clip length.
func DurationCap(configured time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}

	return MaxDuration
}
