// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"strconv"
	"strings"
)

const (
	// bytesPerUnit is the step between decimal units.
	bytesPerUnit = 1000
	// sizeDecimals is how many decimals a size above a kilobyte carries.
	sizeDecimals = 1
	// sizeBits is the precision a size is formatted from.
	sizeBits = 64
	// decimalBase is the base a byte count is written in.
	decimalBase = 10
)

// byteUnits are the decimal units a size is written in, after bytes.
var byteUnits = []string{"kB", "MB", "GB", "TB", "PB"}

// FormatBytes writes a size in decimal units.
//
// Sizes under a kilobyte are counted in bytes. Larger ones take one decimal,
// left out when it is zero.
//
// Parameters:
//   - size: The size in bytes.
//
// Returns:
//   - text: Such as "512 bytes", "6 MB", or "18.4 GB".
func FormatBytes(size uint64) string {
	if size < bytesPerUnit {
		return strconv.FormatUint(size, decimalBase) + " bytes"
	}

	value := float64(size)
	unit := ""

	for _, next := range byteUnits {
		if value < bytesPerUnit {
			break
		}

		value /= bytesPerUnit

		unit = next
	}

	text := strconv.FormatFloat(value, 'f', sizeDecimals, sizeBits)

	return strings.TrimSuffix(text, ".0") + " " + unit
}
