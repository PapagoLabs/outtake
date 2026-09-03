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

func TestTimecodeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give float64
		want string
	}{
		{give: 0, want: "00:00:00.000"},
		{give: 10.5, want: "00:00:10.500"},
		{give: 90, want: "00:01:30.000"},
		{give: 3723.123, want: "01:02:03.123"},
		{give: -1, want: "00:00:00.000"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, FromSeconds(tt.give).String())
	}
}

func TestTimecodeHMS(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "00:00:11", FromSeconds(10.6).HMS())
	assert.Equal(t, "01:02:03", FromSeconds(3723.4).HMS())
}

func TestParseTimecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give string
		want time.Duration
	}{
		{give: "", want: 0},
		{give: "10.5", want: 10500 * time.Millisecond},
		{give: "10.5s", want: 10500 * time.Millisecond},
		{give: "250ms", want: 250 * time.Millisecond},
		{give: "1500us", want: 1500 * time.Microsecond},
		{give: "1:30", want: 90 * time.Second},
		{give: "1:30.250", want: 90250 * time.Millisecond},
		{give: "01:02:03.123", want: time.Hour + 2*time.Minute + 3123*time.Millisecond},
	}

	for _, tt := range tests {
		got, err := Parse(tt.give)
		require.NoError(t, err, tt.give)
		assert.Equal(t, tt.want, got.Duration(), tt.give)
	}
}

func TestParseTimecodeInvalid(t *testing.T) {
	t.Parallel()

	invalid := []string{"nope", "1:-30", "--1s", "+10", "NaN", "Inf", "-Inf"}
	for _, give := range invalid {
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

func TestFromSecondsOverflow(t *testing.T) {
	t.Parallel()

	maxSeconds := float64(math.MaxInt64) / float64(time.Second)
	safe := math.Nextafter(maxSeconds, 0)
	assert.Positive(t, FromSeconds(safe).Duration())
	assert.Equal(t, time.Duration(0), FromSeconds(maxSeconds).Duration())

	over := math.Nextafter(maxSeconds, math.Inf(1))
	assert.Equal(t, time.Duration(0), FromSeconds(over).Duration())
}

func TestParseOverflow(t *testing.T) {
	t.Parallel()

	_, err := Parse("1e20s")
	require.ErrorIs(t, err, ErrInvalidTimecode)

	_, err = Parse("2562048:00:00")
	require.ErrorIs(t, err, ErrInvalidTimecode)
}
