// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMaxDurationIsTenMinutes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 10*time.Minute, MaxDuration)
}

func TestDurationCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give time.Duration
		want time.Duration
	}{
		{
			name: "zero falls back to the maximum",
			give: 0,
			want: MaxDuration,
		},
		{
			name: "one nanosecond below zero falls back to the maximum",
			give: -time.Nanosecond,
			want: MaxDuration,
		},
		{
			name: "a negative cap falls back to the maximum",
			give: -time.Hour,
			want: MaxDuration,
		},
		{
			name: "the smallest positive cap is returned as configured",
			give: time.Nanosecond,
			want: time.Nanosecond,
		},
		{
			name: "a positive cap is returned as configured",
			give: 45 * time.Second,
			want: 45 * time.Second,
		},
		{
			name: "the maximum is returned unchanged",
			give: MaxDuration,
			want: MaxDuration,
		},
		{
			name: "a cap above the maximum is returned unchanged",
			give: 2 * MaxDuration,
			want: 2 * MaxDuration,
		},
		{
			name: "the largest representable duration is returned unchanged",
			give: time.Duration(math.MaxInt64),
			want: time.Duration(math.MaxInt64),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, DurationCap(test.give))
		})
	}
}
