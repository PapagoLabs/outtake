// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatSeconds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give float64
		want string
	}{
		{
			name: "whole seconds keep three digits",
			give: 12,
			want: "12.000",
		},
		{
			name: "milliseconds survive",
			give: 12.345,
			want: "12.345",
		},
		{
			name: "end mark of a sub-minute clip survives",
			give: 8.007,
			want: "8.007",
		},
		{
			name: "float noise below a millisecond is dropped",
			give: 12.3450000001,
			want: "12.345",
		},
		{
			name: "sub-millisecond value rounds down to a third",
			give: 0.0004,
			want: "0.000",
		},
		{
			name: "sub-millisecond value rounds up to a fourth",
			give: 0.0006,
			want: "0.001",
		},
		{
			name: "zero is stable",
			give: 0,
			want: "0.000",
		},
		{
			name: "hour scale keeps milliseconds",
			give: 3723.456,
			want: "3723.456",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, FormatSeconds(test.give))
		})
	}
}

// TestFormatSecondsRoundTrips guards the contract the preview redirect depends
// on: the string written into the query must parse back to the same millisecond
// the user chose, otherwise every preview click moves the marks.
func TestFormatSecondsRoundTrips(t *testing.T) {
	t.Parallel()

	marks := []float64{0, 0.001, 1.005, 8.007, 12.345, 61.999, 3723.456, 7199.994}

	for _, mark := range marks {
		parsed, err := strconv.ParseFloat(FormatSeconds(mark), bitRateBits)
		require.NoError(t, err)

		assert.InDelta(t, mark, parsed, 0.0005, "mark %v did not round trip", mark)
	}
}
