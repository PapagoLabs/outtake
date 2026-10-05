// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectionBoundedLength(t *testing.T) {
	t.Parallel()

	length := 23*time.Second + 846*time.Millisecond

	assert.Zero(t, BoundedLength(TypeScreenshot, length),
		"a screenshot renders one frame, so it has no range to bound")
	assert.Equal(t, length, BoundedLength(TypeClip, length))
	assert.Equal(t, length, BoundedLength(TypeGIF, length))
}

func TestSelectionValidateLength(t *testing.T) {
	t.Parallel()

	const cap10m = 10 * time.Minute

	tests := []struct {
		name    string
		kind    Type
		length  time.Duration
		wantErr string
	}{
		{name: "a range inside the cap", kind: TypeClip, length: 15 * time.Second},
		{
			name:    "a zero length range is rejected",
			kind:    TypeClip,
			wantErr: "the range must be longer than zero",
		},
		{
			name:    "a negative range is rejected",
			kind:    TypeClip,
			length:  -time.Second,
			wantErr: "the range must be longer than zero",
		},
		{
			name:    "a range over the cap is rejected",
			kind:    TypeClip,
			length:  cap10m + time.Second,
			wantErr: "must be between 0 and 600 seconds",
		},
		{
			name:   "a range exactly at the cap is accepted",
			kind:   TypeClip,
			length: cap10m,
		},
		{
			name:   "a screenshot has no range, so a zero length is accepted",
			kind:   TypeScreenshot,
			length: 0,
		},
		{
			name:    "a negative screenshot length is rejected",
			kind:    TypeScreenshot,
			length:  -time.Second,
			wantErr: "must be zero or greater",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			selection := Selection{Length: test.length}

			err := selection.Validate(test.kind, cap10m)

			if test.wantErr == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, ErrInvalidDuration)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestSelectionWithinSource(t *testing.T) {
	t.Parallel()

	const film = 2 * time.Hour

	tests := []struct {
		name      string
		start     time.Duration
		length    time.Duration
		source    time.Duration
		wantError bool
	}{
		{
			name:   "a range inside the film is accepted",
			start:  30 * time.Second,
			length: 20 * time.Second,
			source: film,
		},
		{
			name:   "a range ending exactly at the end is accepted",
			start:  100 * time.Second,
			length: film - 100*time.Second,
			source: film,
		},
		{
			name:      "a start past the end is rejected",
			start:     11 * time.Hour,
			length:    20 * time.Second,
			source:    film,
			wantError: true,
		},
		{
			name:      "a range reaching past the end is rejected",
			start:     time.Hour,
			length:    time.Hour + time.Second,
			source:    film,
			wantError: true,
		},
		{
			name:   "a start one second before the end is accepted",
			start:  film - time.Second,
			length: time.Second,
			source: film,
		},
		{
			name:      "a negative start is rejected",
			start:     -time.Second,
			length:    10 * time.Second,
			source:    film,
			wantError: true,
		},
		{
			name:      "a screenshot is bounded by its start alone",
			start:     film,
			source:    film,
			wantError: true,
		},
		{
			name:   "an unprobed source bounds nothing",
			start:  11 * time.Hour,
			length: 20 * time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			selection := Selection{
				Start:        test.start,
				Length:       test.length,
				SourceLength: test.source,
			}

			err := selection.WithinSource(TypeClip)

			if test.wantError {
				require.ErrorIs(t, err, ErrRangeOutsideMedia)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestSelectionWithinSourceNamesTheBounds(t *testing.T) {
	t.Parallel()

	err := Selection{Start: 11 * time.Hour, Length: 20 * time.Second, SourceLength: 2 * time.Hour}.
		WithinSource(TypeClip)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "11hr", "the message names the start the user typed")
	assert.Contains(t, err.Error(), "2hr", "the message names the source's real length")

	overrun := Selection{
		Start:        time.Hour,
		Length:       time.Hour + time.Second,
		SourceLength: 2 * time.Hour,
	}.WithinSource(TypeClip)
	require.Error(t, overrun)

	assert.Contains(t, overrun.Error(), "2hr1s", "the message names the end that overran")
}

func TestSelectionIgnoresAnEndAScreenshotNeverRenders(t *testing.T) {
	t.Parallel()

	selection := Selection{
		Start:        8000 * time.Second,
		Length:       23846 * time.Millisecond,
		SourceLength: 8013846 * time.Millisecond,
	}

	require.Error(t, selection.WithinSource(TypeClip),
		"a range reaching past the end is out of bounds for a clip")
	require.NoError(t, selection.WithinSource(TypeScreenshot),
		"a screenshot is a single frame at the start, so the end mark is not its concern")
}
