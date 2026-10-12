// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFormatBytes covers how the storage page writes a size.
func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := map[uint64]string{
		0:                 "0 bytes",
		512:               "512 bytes",
		1000:              "1 kB",
		6_000_000:         "6 MB",
		410_260_000:       "410.3 MB",
		18_400_000_000:    "18.4 GB",
		931_000_000_000:   "931 GB",
		2_500_000_000_000: "2.5 TB",
	}

	for size, want := range tests {
		assert.Equal(t, want, FormatBytes(size), "%+v", size)
	}
}
