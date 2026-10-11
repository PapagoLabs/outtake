// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
)

const (
	// barDecimals is how many decimal places a bar's width is written with.
	barDecimals = 1
	// barBits is the size of the float a bar's width is written from.
	barBits = 64
)

// RemainingLabel says how long an encode has left, from the rate it has had
// since it started.
//
// Parameters:
//   - started: When the encode began, zero when it has not.
//   - now: The time to estimate from.
//   - percent: How far the encode has got, from 0 to 100.
//
// Returns:
//   - label: Such as "2m 34s left", or empty when there is no
//     estimate yet.
func RemainingLabel(started, now time.Time, percent int) string {
	left, ok := clip.Remaining(started, now, percent)
	if !ok {
		return ""
	}

	return remainingText(left)
}

// remainingText writes a time left in compact units, to the second: minutes
// and seconds under an hour, and hours and minutes past it, leaving out any
// part that is zero.
//
// Parameters:
//   - left: The time left.
//
// Returns:
//   - text: Such as "35s left", "2m 34s left", or "1h 20m left".
func remainingText(left time.Duration) string {
	rounded := max(left.Round(time.Second), time.Second)

	hours := int(rounded / time.Hour)
	minutes := int(rounded % time.Hour / time.Minute)
	seconds := int(rounded % time.Minute / time.Second)

	var parts []string

	if hours > 0 {
		parts = append(parts, strconv.Itoa(hours)+"h")
	}

	if minutes > 0 {
		parts = append(parts, strconv.Itoa(minutes)+"m")
	}

	if seconds > 0 && hours == 0 {
		parts = append(parts, strconv.Itoa(seconds)+"s")
	}

	return strings.Join(parts, " ") + " left"
}

// BarStyle returns the inline style that sizes a progress bar.
//
// The width is written to a tenth of a percent, so a slow encode's bar still
// moves between polls.
//
// Parameters:
//   - percent: How far to draw the bar, from 0 to 100.
//
// Returns:
//   - style: The bar's inline style, such as "width: 42.6%".
func BarStyle(percent float64) string {
	return "width: " + strconv.FormatFloat(percent, 'f', barDecimals, barBits) + "%"
}
