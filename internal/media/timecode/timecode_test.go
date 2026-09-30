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

// TestShort pins the short form used on the metadata lines.
//
// HH:MM:SS spends most of its width on zeros, so a short clip should read as
// "20s" and an hour and twenty seconds as "1hr20s".
func TestShort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		seconds float64
		want    string
	}{
		{name: "zero still reads as a value", seconds: 0, want: "0s"},
		{name: "seconds only", seconds: 20, want: "20s"},
		{name: "rounds to the nearest second", seconds: 20.6, want: "21s"},
		{name: "minutes and seconds", seconds: 80, want: "1min20s"},
		{name: "whole minutes drop the seconds", seconds: 120, want: "2min"},
		{name: "a whole hour drops the rest", seconds: 3600, want: "1hr"},
		{name: "hours and seconds skip the empty minutes", seconds: 3620, want: "1hr20s"},
		{name: "hours and minutes skip the empty seconds", seconds: 3660, want: "1hr1min"},
		{name: "all three parts", seconds: 7325, want: "2hr2min5s"},
		{name: "a long runtime stays short", seconds: 7845, want: "2hr10min45s"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, FromSeconds(test.seconds).Short())
		})
	}
}

// TestShortClampsNegativeDurations covers a timecode before zero, which can
// arrive from a mark the user typed badly.
func TestShortClampsNegativeDurations(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0s", FromSeconds(-30).Short())
}
