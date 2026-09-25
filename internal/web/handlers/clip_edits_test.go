// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/queue"
)

func TestApplyClipEditsPreservesOmittedWebSafeColor(t *testing.T) {
	t.Parallel()

	job := testClipJob("clip-1", queue.JobTypeClip)

	job.WebSafeColor = true

	applyClipEdits(job, api.ClipRequest{
		StartTime: 1,
		Duration:  5,
	})

	assert.True(t, job.WebSafeColor)

	off := false
	applyClipEdits(job, api.ClipRequest{
		StartTime:    1,
		Duration:     5,
		WebSafeColor: &off,
	})

	assert.False(t, job.WebSafeColor)
}

// TestPreviewRedirectKeepsMillisecondMarks guards the precision contract for
// the preview round trip. The redirect is the only thing carrying the user's
// start and end back to the form, so a coarser format silently moves both marks
// on every click.
func TestPreviewRedirectKeepsMillisecondMarks(t *testing.T) {
	t.Parallel()

	location := previewRedirectURL("42", "preview-1", 12.345, 18.007, new(true), exportFormState{})

	parsed, err := url.Parse(location)
	require.NoError(t, err)

	assert.Equal(t, "/media/item/42", parsed.Path)

	query := parsed.Query()
	assert.Equal(t, "preview-1", query.Get("preview"))
	assert.Equal(t, "12.345", query.Get("start"))
	assert.Equal(t, "18.007", query.Get("end"))
	assert.Equal(t, webSafeQueryValue(new(true)), query.Get(queryWebSafeColor))
}

// TestPreviewRedirectMarksParseBack walks the full hand-off the browser
// performs: it reads start and end back with ParseFloat and re-renders the
// form, so the parsed values have to match the marks that were submitted.
func TestPreviewRedirectMarksParseBack(t *testing.T) {
	t.Parallel()

	marks := []struct {
		name  string
		start float64
		end   float64
	}{
		{name: "sub-second start", start: 0.007, end: 4.251},
		{name: "minute boundary", start: 61.999, end: 75.001},
		{name: "hour scale", start: 3723.456, end: 3728.994},
	}

	for _, test := range marks {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			location := previewRedirectURL(
				"7",
				"p",
				test.start,
				test.end,
				new(false),
				exportFormState{},
			)

			parsed, err := url.Parse(location)
			require.NoError(t, err)

			start, err := strconv.ParseFloat(parsed.Query().Get("start"), floatBitSize)
			require.NoError(t, err)

			end, err := strconv.ParseFloat(parsed.Query().Get("end"), floatBitSize)
			require.NoError(t, err)

			assert.InDelta(t, test.start, start, 0.0005)
			assert.InDelta(t, test.end, end, 0.0005)
			assert.Greater(t, end, start, "end must stay ahead of start")
		})
	}
}

func TestDerefBool(t *testing.T) {
	t.Parallel()

	assert.False(t, derefBool(nil))
	assert.True(t, derefBool(new(true)))
	assert.False(t, derefBool(new(false)))
}
