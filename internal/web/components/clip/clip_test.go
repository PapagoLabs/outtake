// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestClipCard(t *testing.T) {
	t.Parallel()

	base := view.ClipItem{
		ID:          "c1",
		Name:        "Intro",
		MediaID:     "42",
		MediaTitle:  "Movie",
		ClipType:    "clip",
		Status:      view.ClipStatusCompleted,
		Progress:    100,
		CreatedAt:   "2026-01-01T00:00:00Z",
		StartTime:   1,
		Duration:    5,
		Quality:     "archive",
		ProfileName: "Archive",
		Profiles: []view.ClipProfileOption{
			{ID: "archive", Name: "Archive", IsDefault: false},
		},
		FileExists:    true,
		AudioIndex:    0,
		AudioTracks:   nil,
		CropBlackBars: false,
	}

	tests := []struct {
		name        string
		tweak       func(*view.ClipItem)
		contains    []string
		notContains []string
	}{
		{
			name: "completed with file",
			contains: []string{
				"Archive",
				"Video clip",
				"/clips/c1/file",
				"Regenerate",
				"<details",
				"Preview",
				"<video",
				"hx-confirm",
			},
			notContains: []string{"<details open", "On disk"},
		},
		{
			name: "falls back to media title",
			tweak: func(item *view.ClipItem) {
				item.Name = ""
			},
			contains: []string{
				"Movie",
				`name="name"`,
				`value="Movie"`,
			},
		},
		{
			name: "missing file",
			tweak: func(item *view.ClipItem) {
				item.FileExists = false
			},
			contains: []string{"Missing file"},
			notContains: []string{
				"On disk",
				"<video",
				"<details",
			},
		},
		{
			name: "failed status",
			tweak: func(item *view.ClipItem) {
				item.Status = "failed"
				item.FileExists = false
				item.Error = "ffmpeg exited 1"
			},
			contains:    []string{"failed", "Missing file", "ffmpeg exited 1"},
			notContains: []string{"On disk", "hx-trigger"},
		},
		{
			name: "canceled status",
			tweak: func(item *view.ClipItem) {
				item.Status = view.ClipStatusCancelled
				item.FileExists = false
			},
			contains: []string{view.ClipStatusCancelled},
			notContains: []string{
				"Missing file",
				"On disk",
				"hx-trigger",
			},
		},
		{
			name: "gif preview",
			tweak: func(item *view.ClipItem) {
				item.ClipType = "gif"
			},
			contains: []string{
				"<img",
				"/clips/c1/file",
				"Preview",
			},
			notContains: []string{"<video"},
		},
		{
			name: "screenshot preview",
			tweak: func(item *view.ClipItem) {
				item.ClipType = "screenshot"
			},
			contains: []string{
				"<img",
				"/clips/c1/file",
			},
			notContains: []string{"<video"},
		},
		{
			name: "active polling hides preview",
			tweak: func(item *view.ClipItem) {
				item.Status = view.ClipStatusProcessing
				item.Progress = 40
				item.FileExists = false
			},
			contains: []string{
				`hx-get="/clips/c1/row"`,
				`hx-trigger="every 2s"`,
				"40%",
				"Cancel",
			},
			notContains: []string{
				"Preview",
				"<video",
				"<img",
				"On disk",
				"Missing file",
			},
		},
		{
			name: "audio tracks",
			tweak: func(item *view.ClipItem) {
				item.AudioTracks = []view.AudioTrackOption{
					{Index: 0, Label: "eng · aac"},
				}
			},
			contains: []string{`name="audioIndex"`, "eng · aac"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			item := base
			if test.tweak != nil {
				test.tweak(&item)
			}

			var buf strings.Builder

			err := ClipCard(item).Render(t.Context(), &buf)
			require.NoError(t, err)

			body := buf.String()
			for _, want := range test.contains {
				assert.Contains(t, body, want)
			}

			for _, hide := range test.notContains {
				assert.NotContains(t, body, hide)
			}
		})
	}
}
