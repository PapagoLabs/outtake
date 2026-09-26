// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/queue"
)

// TestApplyClipEditsPersistsGIFDimensions covers the seam where the edit is
// actually lost: the form handler copies the new dimensions onto the job, and
// SaveClip has to keep them.
//
// A GIF resized through the edit form used to save successfully while the stored
// row kept the original size, so the form reverted and a regenerate re-encoded
// at the old dimensions.
func TestApplyClipEditsPersistsGIFDimensions(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := testClipJob("gif-1", queue.JobTypeGIF)

	job.Width = 1280
	job.FPS = 24
	require.NoError(t, db.SaveClip(t.Context(), job))

	// The edit form posts the same clip with a smaller size.
	applyClipEdits(job, api.ClipRequest{
		MediaID:    job.MediaID,
		ClipType:   string(job.Type),
		StartTime:  job.StartTime,
		Duration:   job.Duration,
		Width:      720,
		FPS:        12,
		AudioIndex: job.AudioIndex,
	})
	require.NoError(t, db.SaveClip(t.Context(), job))

	stored, err := db.GetClip(t.Context(), "gif-1")
	require.NoError(t, err)
	assert.Equal(t, 720, stored.Width, "the resized width must reach the database")
	assert.Equal(t, 12, stored.FPS, "the resized frame rate must reach the database")
}

// TestApplyClipEditsPersistsStartTime guards the neighboring timecode columns,
// which were already in the conflict clause and must stay there.
func TestApplyClipEditsPersistsStartTime(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/clips-time.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := testClipJob("clip-1", queue.JobTypeClip)

	job.StartTime = 10
	job.Duration = 15
	require.NoError(t, db.SaveClip(t.Context(), job))

	applyClipEdits(job, api.ClipRequest{
		MediaID:   job.MediaID,
		ClipType:  string(job.Type),
		StartTime: 42.345,
		Duration:  8,
	})
	require.NoError(t, db.SaveClip(t.Context(), job))

	stored, err := db.GetClip(t.Context(), "clip-1")
	require.NoError(t, err)
	assert.InDelta(t, 42.345, stored.StartTime, 0.0005)
	assert.InDelta(t, 8, stored.Duration, 0.0005)
}
