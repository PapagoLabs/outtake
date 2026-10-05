// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/clip"
)

func TestFormatClipCreated(t *testing.T) {
	t.Parallel()

	assert.Empty(t, FormatClipCreated(time.Time{}),
		"a clip that was never stamped shows no time at all")
	assert.Equal(
		t,
		"Sep 3, 2026 4:32 AM",
		FormatClipCreated(time.Date(2026, time.September, 3, 4, 32, 20, 0, time.UTC)),
	)
	assert.Equal(
		t,
		"Sep 3, 2026 4:32 PM",
		FormatClipCreated(time.Date(2026, time.September, 3, 16, 32, 20, 0, time.UTC)),
	)
	assert.Equal(
		t,
		"Jan 1, 2026 12:00 AM",
		FormatClipCreated(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)),
		"midnight renders as 12 AM, not 0 AM",
	)
}

func TestClipTypeLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Clip", ClipTypeLabel(clip.TypeClip))
	assert.Equal(t, "GIF", ClipTypeLabel(clip.TypeGIF))
	assert.Equal(t, "Screenshot", ClipTypeLabel(clip.TypeScreenshot))
}

func TestClipItemDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give ClipItem
		want string
	}{
		{
			name: "named clip",
			give: ClipItem{Name: "Intro", MediaTitle: "Movie"},
			want: "Intro",
		},
		{
			name: "falls back to media title",
			give: ClipItem{Name: "", MediaTitle: "Movie"},
			want: "Movie",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			item := test.give
			assert.Equal(t, test.want, item.DisplayName())
		})
	}
}

func TestClipItemIsActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give clip.Status
		want bool
	}{
		{name: string(clip.StatusPending), give: clip.StatusPending, want: true},
		{name: string(clip.StatusProcessing), give: clip.StatusProcessing, want: true},
		{name: string(clip.StatusCompleted), give: clip.StatusCompleted, want: false},
		{name: "failed", give: "failed", want: false},
		{name: "empty", give: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			item := ClipItem{Status: test.give}
			assert.Equal(t, test.want, item.IsActive())
		})
	}
}

func TestClipItemEndTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		giveStart, giveDur, want time.Duration
	}{
		{"unset", 0, 0, 0},
		{"fractional", 1500 * time.Millisecond, 4250 * time.Millisecond, 5750 * time.Millisecond},
		{"whole seconds", 600 * time.Second, time.Second, 601 * time.Second},
		{"past a day", 25 * time.Hour, 999 * time.Millisecond, 25*time.Hour + 999*time.Millisecond},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			item := ClipItem{StartTime: test.giveStart, Duration: test.giveDur}
			assert.Equal(t, test.want, item.EndTime())
		})
	}
}

func TestClipItemGIFDefaults(t *testing.T) {
	t.Parallel()

	unset := ClipItem{}
	assert.Equal(t, 480, unset.GIFWidth())
	assert.Equal(t, 10, unset.GIFFPS())

	set := ClipItem{Width: 640, FPS: 12}
	assert.Equal(t, 640, set.GIFWidth())
	assert.Equal(t, 12, set.GIFFPS())
}

func TestClipItemCanPlay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give ClipItem
		want bool
	}{
		{
			name: "completed with file",
			give: ClipItem{Status: clip.StatusCompleted, FileExists: true},
			want: true,
		},
		{
			name: "completed missing file",
			give: ClipItem{Status: clip.StatusCompleted, FileExists: false},
			want: false,
		},
		{
			name: "processing with file",
			give: ClipItem{Status: clip.StatusProcessing, FileExists: true},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			item := test.give
			assert.Equal(t, test.want, item.CanPlay())
		})
	}
}
