// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
)

func TestApplyClipEditsPreservesOmittedWebSafeColor(t *testing.T) {
	t.Parallel()

	job := testClipJob("clip-1", queue.JobTypeClip)

	job.WebSafeColor = true

	applyClipEdits(job, ClipRequest{
		StartTime: 1,
		Duration:  5,
	})

	assert.True(t, job.WebSafeColor)

	off := false
	applyClipEdits(job, ClipRequest{
		StartTime:    1,
		Duration:     5,
		WebSafeColor: &off,
	})

	assert.False(t, job.WebSafeColor)
}

func TestDerefBool(t *testing.T) {
	t.Parallel()

	assert.False(t, derefBool(nil))
	assert.True(t, derefBool(new(true)))
	assert.False(t, derefBool(new(false)))
}

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
