// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

// BarPercent returns how far a progress bar is drawn.
//
// It takes the exact percent an encode reported while that refines the whole
// percent, so a slow encode's bar moves between whole percents. Otherwise it
// takes the whole percent, such as for a render read back from the database or
// one whose exact value belongs to an earlier encode.
//
// Parameters:
//   - whole: The render's percent complete, rounded down.
//   - exact: The percent the encode last reported, not rounded.
//
// Returns:
//   - percent: How far to draw the bar, from 0 to 100.
func BarPercent(whole int, exact float64) float64 {
	floor := float64(whole)

	if exact >= floor && exact < floor+1 {
		return exact
	}

	return floor
}
