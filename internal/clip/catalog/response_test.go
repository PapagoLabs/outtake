// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/clip"
)

func TestClipResponse(t *testing.T) {
	t.Parallel()

	job := &clip.Job{
		ID:         "c1",
		Name:       "Intro",
		MediaID:    "42",
		MediaTitle: "Movie",
		MediaType:  clip.DefaultMediaType,
		Type:       clip.TypeGIF,

		AudioIndex:    2,
		CropBlackBars: true,
		WebSafeColor:  true,
		PreserveHDR:   true,
		CreatedAt: time.Date(
			2026,
			time.January,
			2,
			3,
			4,
			5,
			0,
			time.UTC,
		),
		Status:     clip.StatusCompleted,
		Progress:   100,
		InputPath:  "/media/movie.mkv",
		OutputPath: "/out/clips/c1.gif",
	}

	response := ClipResponse(job)

	assert.Equal(t, "c1", response.ID)
	assert.Equal(t, "Intro", response.Name)
	assert.Equal(t, "42", response.MediaID)
	assert.Equal(t, "Movie", response.MediaTitle)
	assert.Equal(t, clip.DefaultMediaType, response.MediaType)
	assert.Equal(t, clip.TypeGIF, response.ClipType)
	assert.Equal(t, clip.StatusCompleted, response.Status)
	assert.Equal(t, 100, response.Progress)
	assert.Equal(t, 2, response.AudioIndex)
	assert.True(t, response.CropBlackBars)
	assert.True(t, response.WebSafeColor)
	assert.True(t, response.PreserveHDR)
	assert.Equal(t, job.CreatedAt, response.CreatedAt)
	assert.Empty(t, response.InputPath, "the source path is not part of the public payload")
	assert.Empty(t, response.OutputPath, "the output path is not part of the public payload")
}

func TestClipResponses(t *testing.T) {
	t.Parallel()

	jobs := []*clip.Job{
		{ID: "c1", Type: clip.TypeClip, Status: clip.StatusCompleted},
		{ID: "c2", Type: clip.TypeScreenshot, Status: clip.StatusFailed},
	}

	responses := ClipResponses(jobs)

	assert.Len(t, responses, 2)
	assert.Equal(t, "c1", responses[0].ID)
	assert.Equal(t, clip.TypeScreenshot, responses[1].ClipType)
	assert.Equal(t, clip.StatusFailed, responses[1].Status)

	assert.Empty(t, ClipResponses(nil), "no clips still answers with an empty list, not null")
}
