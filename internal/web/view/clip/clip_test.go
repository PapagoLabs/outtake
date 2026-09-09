// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testMediaTitle = "Movie"

func TestClipTypeLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Video clip", ClipTypeLabel("clip"))
	assert.Equal(t, "GIF", ClipTypeLabel("gif"))
	assert.Equal(t, "Screenshot", ClipTypeLabel("screenshot"))
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
			give: ClipItem{Name: "Intro", MediaTitle: testMediaTitle},
			want: "Intro",
		},
		{
			name: "falls back to media title",
			give: ClipItem{Name: "", MediaTitle: testMediaTitle},
			want: testMediaTitle,
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
		give string
		want bool
	}{
		{name: ClipStatusPending, give: ClipStatusPending, want: true},
		{name: ClipStatusProcessing, give: ClipStatusProcessing, want: true},
		{name: ClipStatusCompleted, give: ClipStatusCompleted, want: false},
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

	item := ClipItem{StartTime: 1.5, Duration: 4.25}
	assert.InDelta(t, 5.75, item.EndTime(), 0.001)
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
			give: ClipItem{Status: ClipStatusCompleted, FileExists: true},
			want: true,
		},
		{
			name: "completed missing file",
			give: ClipItem{Status: ClipStatusCompleted, FileExists: false},
			want: false,
		},
		{
			name: "processing with file",
			give: ClipItem{Status: ClipStatusProcessing, FileExists: true},
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
