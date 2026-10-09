// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
)

// noopJobHandler is a queue worker that renders nothing, for tests that only
// care what the queue does with a job rather than what the render produces.
func noopJobHandler(_ context.Context, _ *clip.Job) error {
	return nil
}

// queueForTest returns a queue holding the given clips.
//
// Parameters:
//   - t: The test the queue belongs to.
//   - jobs: Clips the queue should hold.
//
// Returns:
//   - jobQueue: The queue under test.
func queueForTest(t *testing.T, jobs ...*clip.Job) *queue.Queue {
	t.Helper()

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	for _, job := range jobs {
		jobQueue.Restore(job)
	}

	return jobQueue
}

// testClipJob is a completed clip for tests that overwrite the fields they care
// about and leave the rest alone.
//
// Parameters:
//   - id: Clip id.
//   - kind: Type of clip.
//
// Returns:
//   - job: The clip under test.
func testClipJob(id string, kind clip.Type) *clip.Job {
	return &clip.Job{
		ID:         id,
		Type:       kind,
		Name:       "Intro",
		MediaID:    "42",
		MediaTitle: "Movie",
		MediaType:  clip.DefaultMediaType,

		StartTime:  0,
		Duration:   0,
		Quality:    "profile-1080p",
		AudioIndex: 0,

		CreatedAt: time.Time{},
		UpdatedAt: time.Time{}, InputPath: "/media/test.mkv",

		Status: clip.StatusCompleted,
	}
}

// closeBody closes a response body and fails the test when it cannot.
//
// Parameters:
//   - t: The test the response belongs to.
//   - resp: The response to close.
func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()

	require.NoError(t, resp.Body.Close())
}
