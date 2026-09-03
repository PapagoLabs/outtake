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

func TestClipCardShowsProfileAndFile(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ClipCard(view.ClipItem{
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
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "Archive")
	assert.Contains(t, body, "On disk")
	assert.Contains(t, body, "/clips/c1/file")
	assert.Contains(t, body, "Regenerate")
	assert.Contains(t, body, "<details")
	assert.Contains(t, body, "Preview")
	assert.Contains(t, body, "<video")
	assert.NotContains(t, body, "<details open")
	assert.NotContains(t, body, `name="audioIndex"`)
}

func TestClipCardFallsBackToMediaTitle(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ClipCard(view.ClipItem{
		ID:          "c1",
		Name:        "",
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
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "Movie")
	assert.Contains(t, body, `name="name"`)
	assert.Contains(t, body, `value="Movie"`)
}
