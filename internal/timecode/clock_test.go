// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package timecode

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTimecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give string
		want time.Duration
	}{
		{give: "", want: 0},
		{give: "   ", want: 0},
		{give: "10.5", want: 10500 * time.Millisecond},
		{give: "10.5s", want: 10500 * time.Millisecond},
		{give: "250ms", want: 250 * time.Millisecond},
		{give: "1500us", want: 1500 * time.Microsecond},
		{give: "1:30", want: 90 * time.Second},
		{give: "1:30.250", want: 90250 * time.Millisecond},
		{give: "01:02:03.123", want: time.Hour + 2*time.Minute + 3123*time.Millisecond},
		{give: "-1s", want: -time.Second},
	}

	for _, tt := range tests {
		got, err := Parse(tt.give)
		require.NoError(t, err, tt.give)
		assert.Equal(t, tt.want, got.Duration(), tt.give)
	}
}

func TestParseTimecodeInvalid(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"nope", "1:-30", "--1s", "+10", "NaN", "Inf", "-Inf",
		"1e20s", "1:2:3:4", "aa:bb:cc", "-", "1e400", ":", "1:",
	}

	for _, give := range invalid {
		_, err := Parse(give)
		require.ErrorIs(t, err, ErrInvalidTimecode, give)
	}
}

func TestParseReportsOffendingText(t *testing.T) {
	t.Parallel()

	_, err := Parse("aa:bb:cc")
	require.ErrorIs(t, err, ErrInvalidTimecode)
	assert.ErrorContains(t, err, "aa:bb:cc")
}

func TestParseOverflow(t *testing.T) {
	t.Parallel()

	_, err := Parse("1e20s")
	require.ErrorIs(t, err, ErrInvalidTimecode)

	_, err = Parse("2562048:00:00")
	require.ErrorIs(t, err, ErrInvalidTimecode)
}

func TestParseClockFieldOverflow(t *testing.T) {
	t.Parallel()

	overflowing := []string{
		"9999999999:00:00",
		"0:153722868:00",
		"0:0:99999999999999999999",
		"2562047:60:00",
		"2562047:00:3600",
	}

	for _, give := range overflowing {
		_, err := Parse(give)
		require.ErrorIs(t, err, ErrInvalidTimecode, give)
	}
}

func TestParseClockFieldRejection(t *testing.T) {
	t.Parallel()

	rejected := []string{"x:00:00", "00:x:00", "00:00:x", "1:x", "x:1"}

	for _, give := range rejected {
		_, err := Parse(give)
		require.ErrorIs(t, err, ErrInvalidTimecode, give)
	}
}

func TestFFmpegClockRoundTrip(t *testing.T) {
	t.Parallel()

	clock := FFmpegClock{}
	original := time.Hour + 2*time.Minute + 3*time.Second + 123*time.Millisecond
	parsed, err := clock.Parse(clock.Format(original))
	require.NoError(t, err)
	assert.Equal(t, original, parsed)
}

func TestFFmpegClockFormatClampsNegative(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "00:00:00.000", DefaultClock.Format(-time.Second))
}
