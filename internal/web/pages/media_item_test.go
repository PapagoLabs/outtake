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
		Title:         "Movie",
		Type:          "",
		Duration:      0,
		MaxDur:        600,
		Clips:         nil,
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
	assert.Contains(t, body, `src="/assets/js/media-item.js"`)
	assert.Contains(t, body, `data-max-dur="600"`)
	assert.Contains(t, body, `name="endTime"`)
	assert.Contains(t, body, "Time")
	assert.Contains(t, body, `name="cropBlackBars"`)
	assert.Less(t, strings.Index(body, `id="clipType"`), strings.Index(body, `id="name"`))
	assert.NotContains(t, body, "Start (seconds)")
	assert.NotContains(t, body, "formatTimecode")
}
