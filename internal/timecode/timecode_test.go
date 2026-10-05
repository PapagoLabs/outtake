// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package timecode

import (
	"math"
	"strconv"
	"strings"
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

func TestFromSecondsOverflow(t *testing.T) {
	t.Parallel()

	maxSeconds := float64(math.MaxInt64) / float64(time.Second)
	safe := math.Nextafter(maxSeconds, 0)
	assert.Positive(t, FromSeconds(safe).Duration())
	assert.Equal(t, time.Duration(0), FromSeconds(maxSeconds).Duration())

	over := math.Nextafter(maxSeconds, math.Inf(1))
	assert.Equal(t, time.Duration(0), FromSeconds(over).Duration())
}

func TestFromDurationKeepsSignedValues(t *testing.T) {
	t.Parallel()

	negative := FromDuration(-30 * time.Second)

	assert.Equal(t, -30*time.Second, negative.Duration())
	assert.Equal(t, "00:00:00.000", negative.String())
	assert.Equal(t, "0s", negative.Short())
	assert.Equal(t, 3723456*time.Millisecond, FromDuration(3723456*time.Millisecond).Duration())
}

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

func TestShortClampsNegativeDurations(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0s", FromSeconds(-30).Short())
}

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
		{
			name: "exact millisecond tie rounds away from zero",
			give: 1.0005,
			want: "1.001",
		},
		{
			name: "exact sub-second tie rounds away from zero",
			give: 0.0625,
			want: "0.063",
		},
		{
			name: "negative seconds render as zero",
			give: -1.5,
			want: "0.000",
		},
		{
			name: "NaN renders as zero",
			give: math.NaN(),
			want: "0.000",
		},
		{
			name: "positive infinity renders as zero",
			give: math.Inf(1),
			want: "0.000",
		},
		{
			name: "negative infinity renders as zero",
			give: math.Inf(-1),
			want: "0.000",
		},
		{
			name: "out of range seconds render as zero",
			give: 1e300,
			want: "0.000",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, FormatSeconds(test.give))
		})
	}
}

func TestTimecodeFormatSecondsClampsNegative(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0.000", FromDuration(-30*time.Second).FormatSeconds())
}

func TestTimecodeFormatSecondsMatchesSecondsDelegate(t *testing.T) {
	t.Parallel()

	durations := []time.Duration{
		0,
		time.Millisecond,
		10500 * time.Millisecond,
		time.Second,
		8*time.Second + 7*time.Millisecond,
		12*time.Second + 345*time.Millisecond,
		61*time.Second + 999*time.Millisecond,
		time.Hour,
		3723*time.Second + 456*time.Millisecond,
		7199*time.Second + 994*time.Millisecond,
	}

	for _, duration := range durations {
		assert.Equal(
			t,
			FormatSeconds(duration.Seconds()),
			FromDuration(duration).FormatSeconds(),
			"duration %v rendered differently through the method and the delegate",
			duration,
		)
	}
}

func TestTimecodeFormatSecondsRoundsToNearestMillisecond(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "1.001", FromDuration(1000500*time.Microsecond).FormatSeconds())
	assert.Equal(t, "1.000", FromDuration(1000400*time.Microsecond).FormatSeconds())
}

func TestFormatSecondsRoundTrips(t *testing.T) {
	t.Parallel()

	marks := []float64{0, 0.001, 1.005, 8.007, 12.345, 61.999, 3723.456, 7199.994}

	for _, mark := range marks {
		parsed, err := strconv.ParseFloat(FormatSeconds(mark), secondsBitSize)
		require.NoError(t, err)

		assert.InDelta(t, mark, parsed, 0.0005, "mark %v did not round trip", mark)
	}
}

func TestFormatSecondsMatchesClockMillis(t *testing.T) {
	t.Parallel()

	marks := []float64{
		0, 0.0005, 0.0625, 0.25, 1.0005, 2.0625, 8.0065, 8.0075, 12.345, 61.999, 3723.456,
	}

	for _, mark := range marks {
		t.Run(strconv.FormatFloat(mark, 'f', -1, secondsBitSize), func(t *testing.T) {
			t.Parallel()

			_, clockMillis, clockHasMillis := strings.Cut(
				DefaultClock.Format(FromSeconds(mark).Duration()), ".",
			)
			_, secondsMillis, secondsHasMillis := strings.Cut(FormatSeconds(mark), ".")

			require.True(t, clockHasMillis, "clock format carries no millisecond field")
			require.True(t, secondsHasMillis, "seconds format carries no millisecond field")

			assert.Equal(t, clockMillis, secondsMillis)
		})
	}
}
