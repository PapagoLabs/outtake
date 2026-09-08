// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaItemPageLoadsExternalScript(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:            "42",
		Title:         testMovie,
		Type:          "",
		Duration:      0,
		MaxDur:        600,
		Clips:         nil,
		ClipStatus:    "",
		ClipType:      "gif",
		ClipQuery:     "intro",
		ClipSort:      "name_asc",
		Profiles:      nil,
		AudioTracks:   nil,
		Error:         "",
		PreviewID:     "",
		StartTime:     0,
		EndTime:       0,
		CropBlackBars: false,
		WebSafeColor:  true,
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
		ID:            "42",
		Title:         testMovie,
		Type:          "",
		Duration:      0,
		MaxDur:        600,
		Clips:         nil,
		ClipStatus:    "pending",
		ClipType:      "",
		ClipQuery:     "",
		ClipSort:      "created_desc",
		Profiles:      nil,
		AudioTracks:   nil,
		Error:         "",
		PreviewID:     "",
		StartTime:     0,
		EndTime:       0,
		CropBlackBars: false,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `name="status"`)
	assert.Contains(t, body, `value="pending"`)
	assert.Contains(t, body, "No clips match these filters.")
	assert.NotContains(t, body, "No clips yet.")
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
