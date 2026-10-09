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

// TestClipItemTracksItsSDRVersion covers the card's SDR state: a render in
// its SDR stage shows that stage, and a playable HDR clip that keeps HDR but
// has no SDR version is reported as lacking one, and nothing else is.
func TestClipItemTracksItsSDRVersion(t *testing.T) {
	t.Parallel()

	rendering := ClipItem{Status: clip.StatusProcessing, Stage: clip.StageSDR}
	assert.True(t, rendering.RendersSDRVersion())

	first := ClipItem{Status: clip.StatusProcessing, Stage: clip.StageClip}
	assert.False(t, first.RendersSDRVersion())

	settled := ClipItem{Status: clip.StatusCompleted, Stage: clip.StageSDR}
	assert.False(t, settled.RendersSDRVersion(), "a finished clip has no running stage")

	older := ClipItem{
		ClipType:    clip.TypeClip,
		Status:      clip.StatusCompleted,
		FileExists:  true,
		PreserveHDR: true,
		SourceHDR:   true,
	}
	assert.True(t, older.LacksSDRVersion())

	withSDR := older

	withSDR.SDRExists = true
	assert.False(t, withSDR.LacksSDRVersion())

	converted := older

	converted.PreserveHDR = false
	assert.False(t, converted.LacksSDRVersion())
}

// TestNewClipItemsAsksOnlyWhereAnSDRVersionMayBe covers the clips list: a
// clip that keeps HDR learns whether its SDR version is stored, and no other
// clip is asked.
func TestNewClipItemsAsksOnlyWhereAnSDRVersionMayBe(t *testing.T) {
	t.Parallel()

	keeps := &clip.Job{ID: "a", Type: clip.TypeClip, PreserveHDR: true, OutputPath: "/c/a.mp4"}
	converts := &clip.Job{ID: "b", Type: clip.TypeClip, OutputPath: "/c/b.mp4"}

	var asked []string

	items := NewClipItems([]*clip.Job{keeps, converts}, nil, time.Minute, func(path string) bool {
		asked = append(asked, path)

		return true
	})

	assert.True(t, items[0].SDRExists)
	assert.False(t, items[1].SDRExists)
	assert.Equal(t, []string{"/c/a.mp4", "/c/a.sdr.mp4", "/c/b.mp4"}, asked)
}

// TestClipItemBadgesNameEachFile covers the badge text of a card: each file's
// recorded format, and for a clip with an SDR version recorded before formats
// were, the range alone, since its file is HDR and its version SDR.
func TestClipItemBadgesNameEachFile(t *testing.T) {
	t.Parallel()

	recorded := ClipItem{
		OutputFormat: clip.Format{Width: 3840, Height: 1608, HDR: true},
		SDRFormat:    clip.Format{Width: 1920, Height: 804},
	}
	assert.Equal(t, "HDR · 4K", recorded.HDRBadge())
	assert.Equal(t, "SDR · 1080p", recorded.SDRBadge())
	assert.Equal(t, "HDR · 4K", recorded.FileBadge())

	older := ClipItem{}
	assert.Equal(t, "HDR", older.HDRBadge())
	assert.Equal(t, "SDR", older.SDRBadge())
	assert.Empty(t, older.FileBadge(), "an older clip's own file is not named")
}
