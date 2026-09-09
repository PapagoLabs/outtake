// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/clip/storage"
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

func TestAssignOutputPaths(t *testing.T) {
	t.Parallel()

	store, err := storage.NewStorage(t.TempDir())
	require.NoError(t, err)

	shot := testClipJob("s1", queue.JobTypeScreenshot)
	AssignOutputPaths(shot, store)
	assert.Contains(t, shot.OutputPath, "screenshots")
	assert.Contains(t, shot.OutputPath, ".jpg")

	gif := testClipJob("g1", queue.JobTypeGIF)
	AssignOutputPaths(gif, store)
	assert.Contains(t, gif.OutputPath, "gifs")
}
