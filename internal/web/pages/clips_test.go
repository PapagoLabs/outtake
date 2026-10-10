// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestClipsToolbar(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Clips(ClipsProps{
		Items:  nil,
		Status: clip.StatusPending,
		Type:   clip.TypeGIF,
		Query:  "intro",
		Sort:   "name_asc",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `hx-get="/clips"`)
	assert.Contains(t, body, `hx-target="#clip-list"`)
	assert.Contains(t, body, `hx-push-url="true"`)
	assert.Contains(t, body, `name="status"`)
	assert.Contains(t, body, `value="pending"`)
	assert.Contains(t, body, `id="clip-list"`)
	assert.Contains(t, body, "No clips match these filters")
	assert.NotContains(t, body, "No clips yet")
}

func TestClipsListOmitsLayout(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ClipsList(ClipsProps{
		Items:  nil,
		Status: "",
		Type:   clip.TypeClip,
		Query:  "",
		Sort:   "created_desc",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "No clips match these filters")
	assert.NotContains(t, body, "Outtake")
	assert.NotContains(t, body, `id="clip-list-type"`)
}

func TestClipsEmptyStateWithoutFilters(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Clips(ClipsProps{
		Items:  nil,
		Status: "",
		Type:   "",
		Query:  "",
		Sort:   "created_desc",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "No clips yet. Choose a title in Media Libraries to make one.")
	assert.Contains(t, body, ">Browse Media</a>")
	assert.NotContains(t, body, "No clips match these filters")
	assert.NotContains(t, body, "Showing ",
		"no status is active, so no filter banner is shown")
}

func TestClipsAnnouncesTheActiveStatus(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Clips(ClipsProps{
		Items:  nil,
		Status: clip.StatusPending,
		Type:   "",
		Query:  "",
		Sort:   "created_desc",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "Showing pending clips")
	assert.Contains(t, body, ">Show All</a>")
	assert.Contains(t, body, "New From Library")
}

func TestClipsListRendersEveryClip(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ClipsList(ClipsProps{
		Items: []view.ClipItem{
			{ID: "clip-one", Name: "Opening"},
			{ID: "clip-two", Name: "Closing"},
		},
		Sort: "created_desc",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `id="clip-clip-one"`)
	assert.Contains(t, body, `id="clip-clip-two"`)
	assert.Contains(t, body, "Opening")
	assert.Contains(t, body, "Closing")
	assert.NotContains(t, body, "No clips yet")
	assert.NotContains(t, body, "No clips match these filters")
}
