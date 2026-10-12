// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	clippreview "github.com/PapagoLabs/outtake/internal/clip/preview"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/progress"
)

// seesEverything is the scope of the owner or an admin, who sees every
// preview.
var seesEverything = clip.Scope{Owner: 42, Everyone: true}

// TestLivePreviewFollowsOnlyAPreviewID covers a value that names no preview.
//
// An empty or overlong value starts no feed.
func TestLivePreviewFollowsOnlyAPreviewID(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 1)

	assert.Nil(t, handler.LivePreview("", seesEverything))
	assert.Nil(t, handler.LivePreview(strings.Repeat("a", maxLivePreviewID+1), seesEverything))
}

// TestLivePreviewReportsEachChange covers the status the feed reports.
//
// The first check reports the status in the shape the status route answers
// with. An unchanged status reports nothing, and new progress reports it
// again.
func TestLivePreviewReportsEachChange(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 1)

	report := make(chan func(float64), 1)
	release := make(chan struct{})

	require.Equal(t, clippreview.AdmittedRender,
		handler.previews.Submit(t.Context(), "p1", func(ctx context.Context) error {
			report <- progress.From(ctx)

			<-release

			return nil
		}))

	setProgress := <-report

	setProgress(20)

	check := handler.LivePreview("p1", seesEverything)
	require.NotNil(t, check)

	first := check(t.Context())
	require.Len(t, first, 1)

	var state statusResponse

	require.NoError(t, json.Unmarshal([]byte(first[0]), &state))
	assert.Equal(t, "p1", state.ID)
	assert.Equal(t, "processing", state.Status)
	assert.Equal(t, 20, state.Progress)

	assert.Empty(t, check(t.Context()), "an unchanged status is not sent again")

	setProgress(30.5)

	moved := check(t.Context())
	require.Len(t, moved, 1)
	require.NoError(t, json.Unmarshal([]byte(moved[0]), &state))
	assert.InDelta(t, 30.5, state.BarPercent, 1e-9)

	close(release)
	awaitTerminal(t, handler.previews, "p1")
}

// TestLivePreviewReportsAGonePreviewOnce covers a preview no longer
// registered.
//
// The page's own poll, which would learn that from a 404, is held back while
// the stream delivers, so the feed says so once.
func TestLivePreviewReportsAGonePreviewOnce(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 1)

	check := handler.LivePreview("never-registered", seesEverything)
	require.NotNil(t, check)

	assert.Equal(t, []string{`{"id":"never-registered","gone":true}`}, check(t.Context()))
	assert.Empty(t, check(t.Context()), "a gone preview is reported once")
}
