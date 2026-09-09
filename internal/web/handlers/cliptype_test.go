// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/storage"
)

func testClipJob(id string, jobType queue.JobType) *queue.Job {
	return &queue.Job{
		ID:            id,
		Type:          jobType,
		Name:          "",
		MediaID:       "",
		MediaTitle:    "",
		MediaType:     "",
		InputPath:     "",
		OutputPath:    "",
		StartTime:     0,
		Duration:      0,
		Quality:       "",
		Width:         0,
		FPS:           0,
		AudioIndex:    0,
		CropBlackBars: false,
		Status:        "",
		Progress:      0,
		Error:         "",
		CreatedAt:     time.Time{},
		UpdatedAt:     time.Time{},
	}
}

func TestNormalizeClipType(t *testing.T) {
	t.Parallel()

	jobType, ok := NormalizeClipType("video")
	require.True(t, ok)
	assert.Equal(t, queue.JobTypeClip, jobType)

	jobType, ok = NormalizeClipType("screenshot")
	require.True(t, ok)
	assert.Equal(t, queue.JobTypeScreenshot, jobType)

	_, ok = NormalizeClipType("nope")
	assert.False(t, ok)
}

func TestClipJobType(t *testing.T) {
	t.Parallel()

	got, err := clipJobType("", queue.JobTypeClip)
	require.NoError(t, err)
	assert.Equal(t, queue.JobTypeClip, got)

	got, err = clipJobType("gif", queue.JobTypeClip)
	require.NoError(t, err)
	assert.Equal(t, queue.JobTypeGIF, got)

	_, err = clipJobType("nope", queue.JobTypeClip)
	require.ErrorIs(t, err, errInvalidClipType)
}

func TestValidateGIFParams(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validateGIFParams(queue.JobTypeClip, 10, 1))
	assert.NoError(t, validateGIFParams(queue.JobTypeGIF, 0, 0))
	assert.NoError(t, validateGIFParams(queue.JobTypeGIF, 480, 10))
	require.ErrorIs(t, validateGIFParams(queue.JobTypeGIF, 50, 10), errInvalidGIFWidth)
	require.ErrorIs(t, validateGIFParams(queue.JobTypeGIF, 480, 60), errInvalidGIFFPS)
}

func TestAssignOutputPaths(t *testing.T) {
	t.Parallel()

	store, err := storage.NewStorage(t.TempDir())
	require.NoError(t, err)

	shot := testClipJob("s1", queue.JobTypeScreenshot)
	assignOutputPaths(shot, store)
	assert.Contains(t, shot.OutputPath, "screenshots")
	assert.Contains(t, shot.OutputPath, ".jpg")

	gif := testClipJob("g1", queue.JobTypeGIF)
	assignOutputPaths(gif, store)
	assert.Contains(t, gif.OutputPath, "gifs")
}
