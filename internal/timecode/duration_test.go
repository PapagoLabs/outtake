// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package timecode

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationFromFloat(t *testing.T) {
	t.Parallel()

	maxQuantity := float64(math.MaxInt64) / float64(time.Second)

	tests := []struct {
		name     string
		quantity float64
		unit     time.Duration
		want     time.Duration
		wantErr  bool
	}{
		{name: "whole quantity", quantity: 2, unit: time.Second, want: 2 * time.Second},
		{
			name:     "fractional quantity",
			quantity: 1.5,
			unit:     time.Second,
			want:     1500 * time.Millisecond,
		},
		{name: "zero quantity", quantity: 0, unit: time.Second, want: 0},
		{
			name:     "millisecond unit",
			quantity: 250,
			unit:     time.Millisecond,
			want:     250 * time.Millisecond,
		},
		{name: "negative quantity", quantity: -1, unit: time.Second, wantErr: true},
		{name: "NaN quantity", quantity: math.NaN(), unit: time.Second, wantErr: true},
		{
			name:     "positive infinite quantity",
			quantity: math.Inf(1),
			unit:     time.Second,
			wantErr:  true,
		},
		{
			name:     "negative infinite quantity",
			quantity: math.Inf(-1),
			unit:     time.Second,
			wantErr:  true,
		},
		{name: "zero unit", quantity: 1, unit: 0, wantErr: true},
		{name: "negative unit", quantity: 1, unit: -time.Second, wantErr: true},
		{
			name:     "quantity at the range limit",
			quantity: maxQuantity,
			unit:     time.Second,
			wantErr:  true,
		},
		{
			name:     "quantity rounding past the range limit",
			quantity: maxQuantity * (1 - 1e-17),
			unit:     time.Second,
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := durationFromFloat(test.quantity, test.unit)

			if test.wantErr {
				require.ErrorIs(t, err, ErrInvalidTimecode)
				assert.Zero(t, got, "a rejected quantity must not carry a duration")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestScaleDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		count   int
		unit    time.Duration
		want    time.Duration
		wantErr bool
	}{
		{name: "zero count", count: 0, unit: time.Hour, want: 0},
		{name: "whole count", count: 2, unit: time.Minute, want: 2 * time.Minute},
		{name: "negative count", count: -1, unit: time.Second, wantErr: true},
		{name: "zero unit", count: 1, unit: 0, wantErr: true},
		{name: "negative unit", count: 1, unit: -time.Second, wantErr: true},
		{name: "count overflows the unit", count: math.MaxInt64, unit: time.Second, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := scaleDuration(test.count, test.unit)

			if test.wantErr {
				require.ErrorIs(t, err, ErrInvalidTimecode)
				assert.Zero(t, got, "a rejected count must not carry a duration")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestAddDuration(t *testing.T) {
	t.Parallel()

	longest := time.Duration(math.MaxInt64)

	tests := []struct {
		name    string
		left    time.Duration
		right   time.Duration
		want    time.Duration
		wantErr bool
	}{
		{name: "sum", left: time.Second, right: 2 * time.Second, want: 3 * time.Second},
		{name: "zero right", left: longest, right: 0, want: longest},
		{name: "negative right", left: longest, right: -time.Second, want: longest - time.Second},
		{name: "sum overflows", left: longest, right: time.Second, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := addDuration(test.left, test.right)

			if test.wantErr {
				require.ErrorIs(t, err, ErrInvalidTimecode)
				assert.Zero(t, got, "a rejected sum must not carry a duration")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}
