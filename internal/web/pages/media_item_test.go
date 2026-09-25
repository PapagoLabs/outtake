// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestMediaItemPageLoadsExternalScript(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:          "42",
		Title:       testMovie,
		Type:        "",
		Duration:    0,
		MaxDur:      600,
		Clips:       nil,
		ClipStatus:  "",
		ClipType:    exportTypeGIF,
		ClipQuery:   "intro",
		ClipSort:    "name_asc",
		Profiles:    nil,
		AudioTracks: nil,
		Error:       "",
		PreviewID:   "",
		StartTime:   0,
		EndTime:     0,
		Export: view.ExportForm{
			WebSafeColor: true,
		},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `src="/assets/js/media-item.js"`)
	assert.Contains(t, body, `data-max-dur="600"`)
	assert.Contains(t, body, `name="endTime"`)
	assert.Contains(t, body, "Time")
	assert.Contains(t, body, `name="cropBlackBars"`)
	assert.Contains(t, body, `name="webSafeColor"`)
	assert.Contains(t, body, "Web-safe color")
	assert.Contains(t, body, `name="webSafeColor" value="1" checked`)
	assert.Less(t, strings.Index(body, `id="clipType"`), strings.Index(body, `id="name"`))
	assert.NotContains(t, body, "Start (seconds)")
	assert.NotContains(t, body, "formatTimecode")
	assert.Contains(t, body, `hx-get="/media/item/42/clips"`)
	assert.Contains(t, body, `hx-target="#item-clip-list"`)
	assert.NotContains(t, body, `hx-push-url`)
	assert.Contains(t, body, `id="clip-list-type"`)
	assert.Contains(t, body, `value="intro"`)
}

func TestMediaItemPagePreservesStatusFilter(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:          "42",
		Title:       testMovie,
		Type:        "",
		Duration:    0,
		MaxDur:      600,
		Clips:       nil,
		ClipStatus:  "pending",
		ClipType:    "",
		ClipQuery:   "",
		ClipSort:    "created_desc",
		Profiles:    nil,
		AudioTracks: nil,
		Error:       "",
		PreviewID:   "",
		StartTime:   0,
		EndTime:     0,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `name="status"`)
	assert.Contains(t, body, `value="pending"`)
	assert.Contains(t, body, "No clips match these filters.")
	assert.NotContains(t, body, "No clips yet.")
}

// TestMediaItemPageRendersCarriedExportForm covers the rendered half of the
// preview round trip. The handlers tests prove the state survives the
// redirect; this proves the form comes back showing it.
func TestMediaItemPageRendersCarriedExportForm(t *testing.T) {
	t.Parallel()

	profiles := []view.ClipProfileOption{
		{ID: "profile-low", Name: "Low", IsDefault: true},
		{ID: "profile-high", Name: "High"},
	}
	tracks := []view.AudioTrackOption{
		{Index: 0, Label: "English"},
		{Index: 2, Label: "Commentary"},
	}

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:          "42",
		Title:       testMovie,
		MaxDur:      600,
		Profiles:    profiles,
		AudioTracks: tracks,
		StartTime:   10.345,
		EndTime:     18.007,
		Export: view.ExportForm{
			Type:          exportTypeGIF,
			Name:          "A named clip",
			Quality:       "profile-high",
			AudioIndex:    2,
			Width:         1280,
			FPS:           24,
			CropBlackBars: true,
			WebSafeColor:  true,
		},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, `<option value="gif" selected>`)
	assert.Contains(t, body, `name="name" placeholder="Optional name" value="A named clip"`)
	assert.Contains(t, body, `<option value="profile-high" selected>`)
	assert.Contains(t, body, `<option value="2" selected>`)
	assert.Contains(t, body, `name="cropBlackBars" value="1" checked`)
	assert.Contains(t, body, `name="webSafeColor" value="1" checked`)
	assert.Contains(t, body, `name="width" type="number" min="120" max="1920" value="1280"`)
	assert.Contains(t, body, `name="fps" type="number" min="5" max="30" value="24"`)
}

// TestMediaItemPageFallsBackToFormDefaults covers a first visit, where nothing
// was carried and the form should show its own defaults rather than blanks.
// TestMediaItemPageFallsBackToFormDefaults covers a first visit, where nothing
// was carried and the form should show its own defaults. The clip name arrives
// already resolved to the media title, because the handler owns that fallback.
func TestMediaItemPageFallsBackToFormDefaults(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  testMovie,
		MaxDur: 600,
		Export: view.ExportForm{Name: testMovie},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, `<option value="clip" selected>`)
	assert.Contains(t, body, `name="name" placeholder="Optional name" value="`+testMovie+`"`)
	assert.Contains(t, body, `name="width" type="number" min="120" max="1920" value="480"`)
	assert.Contains(t, body, `name="fps" type="number" min="5" max="30" value="10"`)
	assert.NotContains(t, body, `name="cropBlackBars" value="1" checked`)
	assert.NotContains(t, body, `name="webSafeColor" value="1" checked`)
}

// TestMediaItemPageRendersClearedName covers a name the user deliberately
// emptied. The form must show it empty rather than falling back to the media
// title, so the next submit stays clear. An input with no value attribute
// submits an empty field, so the attribute is expected to be absent.
func TestMediaItemPageRendersClearedName(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  testMovie,
		MaxDur: 600,
		Export: view.ExportForm{Name: ""},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, `name="name" placeholder="Optional name" class=`)
	assert.NotContains(t, body, `name="name" placeholder="Optional name" value=`)
	assert.Contains(t, body, `name="mediaTitle" value="`+testMovie+`"`)
}

func TestItemClipListOmitsLayout(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ItemClipList(nil, true).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "No clips match these filters.")
	assert.NotContains(t, body, "Outtake")
	assert.NotContains(t, body, `id="clip-list-type"`)
}
