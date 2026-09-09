// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"strings"
	"testing"

	viewclip "github.com/PapagoLabs/outtake/internal/web/view/clip"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClipCard(t *testing.T) {
	t.Parallel()

	base := viewclip.ClipItem{
		ID:          "c1",
		Name:        "Intro",
		MediaID:     "42",
		MediaTitle:  "Movie",
		ClipType:    "clip",
		Status:      viewclip.ClipStatusCompleted,
		Progress:    100,
		CreatedAt:   "2026-01-01T00:00:00Z",
		StartTime:   1,
		Duration:    5,
		Quality:     "archive",
		ProfileName: "Archive",
		Profiles: []viewclip.ClipProfileOption{
			{ID: "archive", Name: "Archive", IsDefault: false},
		},
		FileExists:    true,
		AudioIndex:    0,
		AudioTracks:   nil,
		CropBlackBars: false,
		MaxDur:        600,
	}

	tests := []struct {
		name        string
		tweak       func(*viewclip.ClipItem)
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
				`hx-disable="this"`,
				`name="endTime"`,
				`name="cropBlackBars"`,
				`name="webSafeColor"`,
				`data-max-dur=`,
			},
			notContains: []string{"<details open", "On disk"},
		},
		{
			name: "falls back to media title",
			tweak: func(item *viewclip.ClipItem) {
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
			tweak: func(item *viewclip.ClipItem) {
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
			tweak: func(item *viewclip.ClipItem) {
				item.Status = "failed"
				item.FileExists = false
				item.Error = "ffmpeg exited 1"
			},
			contains:    []string{"failed", "Missing file", "ffmpeg exited 1"},
			notContains: []string{"On disk", "hx-trigger"},
		},
		{
			name: "canceled status",
			tweak: func(item *viewclip.ClipItem) {
				item.Status = viewclip.ClipStatusCancelled
				item.FileExists = false
			},
			contains: []string{viewclip.ClipStatusCancelled},
			notContains: []string{
				"Missing file",
				"On disk",
				"hx-trigger",
			},
		},
		{
			name: "gif preview",
			tweak: func(item *viewclip.ClipItem) {
				item.ClipType = "gif"
				item.Width = 640
				item.FPS = 12
			},
			contains: []string{
				"<img",
				"/clips/c1/file",
				"Preview",
				`name="width"`,
				`name="fps"`,
				`name="endTime"`,
				`value="640"`,
				`value="12"`,
				"GIF width (px)",
				`name="cropBlackBars"`,
			},
			notContains: []string{"<video"},
		},
		{
			name: "screenshot preview",
			tweak: func(item *viewclip.ClipItem) {
				item.ClipType = "screenshot"
			},
			contains: []string{
				"<img",
				"/clips/c1/file",
				"Time",
				`name="startTime"`,
				`name="cropBlackBars"`,
				`data-export-for="screenshot"`,
				`col-start-1 row-start-1`,
			},
			notContains: []string{"<video"},
		},
		{
			name: "active polling hides preview",
			tweak: func(item *viewclip.ClipItem) {
				item.Status = viewclip.ClipStatusProcessing
				item.Progress = 40
				item.FileExists = false
			},
			contains: []string{
				`hx-get="/clips/c1/row"`,
				`hx-trigger="every 2s"`,
				`hx-disable="this"`,
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
			tweak: func(item *viewclip.ClipItem) {
				item.AudioTracks = []viewclip.AudioTrackOption{
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

			typeIdx := strings.Index(body, `name="clipType"`)
			nameIdx := strings.Index(body, `id="name-c1"`)
			if typeIdx >= 0 && nameIdx >= 0 {
				assert.Less(t, typeIdx, nameIdx)
			}
		})
	}
}

func TestListToolbar(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ListToolbar(ListToolbarProps{
		Action:  "/clips",
		Target:  "clip-list",
		PushURL: true,
		Status:  viewclip.ClipStatusCompleted,
		Type:    "gif",
		Query:   "intro",
		Sort:    "name_asc",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `hx-get="/clips"`)
	assert.Contains(t, body, `hx-target="#clip-list"`)
	assert.Contains(t, body, `hx-push-url="true"`)
	assert.Contains(t, body, `hx-trigger="change from:select, submit"`)
	assert.Contains(t, body, `name="status"`)
	assert.Contains(t, body, `value="completed"`)
	assert.Contains(t, body, `name="type"`)
	assert.Contains(t, body, `name="q"`)
	assert.Contains(t, body, `name="sort"`)
	assert.Contains(t, body, `value="intro"`)
	assert.Contains(t, body, "Filter")
}
