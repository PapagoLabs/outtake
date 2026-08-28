// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package queue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testJob(id string, status JobStatus) *Job {
	return &Job{
		ID:         id,
		Type:       JobTypeClip,
		Name:       "",
		MediaID:    "",
		MediaTitle: "",
		MediaType:  "",
		InputPath:  "",
		OutputPath: "",
		StartTime:  0,
		Duration:   0,
		Quality:    "",
		Width:      0,
		FPS:        0,
		Status:     status,
		Progress:   0,
		Error:      "",
		CreatedAt:  time.Time{},
		UpdatedAt:  time.Time{},
	}
}

func TestNewQueue(t *testing.T) {
	t.Parallel()

	q := NewQueue(2, nil)
	assert.Equal(t, 2, q.workers)
	assert.Empty(t, q.jobChan)
}

func TestQueue_SubmitAndRetrieve(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	job := &Job{
		ID:         "test-1",
		Type:       JobTypeClip,
		Name:       "",
		Status:     JobStatusPending,
		MediaID:    "",
		MediaTitle: "",
		MediaType:  "",
		InputPath:  "",
		OutputPath: "",
		StartTime:  0,
		Duration:   0,
		Quality:    "",
		Width:      0,
		FPS:        0,
		Progress:   0,
		Error:      "",
		CreatedAt:  time.Time{},
		UpdatedAt:  time.Time{},
	}

	q.Submit(job)

	retrieved := q.GetJob("test-1")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-1", retrieved.ID)
}

func TestQueue_GetAllJobs(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	q.Submit(&Job{
		ID:         "j1",
		Type:       JobTypeClip,
		Name:       "",
		Status:     JobStatusPending,
		MediaID:    "",
		MediaTitle: "",
		MediaType:  "",
		InputPath:  "",
		OutputPath: "",
		StartTime:  0,
		Duration:   0,
		Quality:    "",
		Width:      0,
		FPS:        0,
		Progress:   0,
		Error:      "",
		CreatedAt:  time.Time{},
		UpdatedAt:  time.Time{},
	})
	q.Submit(&Job{
		ID:         "j2",
		Type:       JobTypeGIF,
		Name:       "",
		Status:     JobStatusPending,
		MediaID:    "",
		MediaTitle: "",
		MediaType:  "",
		InputPath:  "",
		OutputPath: "",
		StartTime:  0,
		Duration:   0,
		Quality:    "",
		Width:      0,
		FPS:        0,
		Progress:   0,
		Error:      "",
		CreatedAt:  time.Time{},
		UpdatedAt:  time.Time{},
	})

	jobs := q.GetAllJobs()
	assert.Len(t, jobs, 2)
}

func TestQueue_ProcessJob_Success(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	completed := false

	handler := func(ctx context.Context, job *Job) error {
		mu.Lock()
		defer mu.Unlock()

		completed = true
		job.Progress = 50

		return nil
	}

	q := NewQueue(1, handler)
	q.Start()

	q.Submit(&Job{
		ID:         "success-job",
		Type:       JobTypeClip,
		Name:       "",
		InputPath:  "/tmp/input.mp4",
		Status:     JobStatusPending,
		MediaID:    "",
		MediaTitle: "",
		MediaType:  "",
		OutputPath: "",
		StartTime:  0,
		Duration:   0,
		Quality:    "",
		Width:      0,
		FPS:        0,
		Progress:   0,
		Error:      "",
		CreatedAt:  time.Time{},
		UpdatedAt:  time.Time{},
	})

	time.Sleep(200 * time.Millisecond)

	job := q.GetJob("success-job")
	require.NotNil(t, job)
	assert.Equal(t, JobStatusCompleted, job.Status)
	assert.Equal(t, 100, job.Progress)

	mu.Lock()
	assert.True(t, completed)
	mu.Unlock()

	q.Stop()
}

func TestQueue_ProcessJob_Failure(t *testing.T) {
	t.Parallel()

	handler := func(ctx context.Context, job *Job) error {
		return assert.AnError
	}

	q := NewQueue(1, handler)
	q.Start()

	q.Submit(&Job{
		ID:         "fail-job",
		Type:       JobTypeGIF,
		Name:       "",
		InputPath:  "/tmp/input.mp4",
		Status:     JobStatusPending,
		MediaID:    "",
		MediaTitle: "",
		MediaType:  "",
		OutputPath: "",
		StartTime:  0,
		Duration:   0,
		Quality:    "",
		Width:      0,
		FPS:        0,
		Progress:   0,
		Error:      "",
		CreatedAt:  time.Time{},
		UpdatedAt:  time.Time{},
	})

	time.Sleep(200 * time.Millisecond)

	job := q.GetJob("fail-job")
	require.NotNil(t, job)
	assert.Equal(t, JobStatusFailed, job.Status)
	assert.NotEmpty(t, job.Error)

	q.Stop()
}

func TestQueue_DeleteAndRestore(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Restore(testJob("kept", JobStatusCompleted))
	q.Restore(testJob("gone", JobStatusCompleted))
	q.Delete("gone")

	assert.NotNil(t, q.GetJob("kept"))
	assert.Nil(t, q.GetJob("gone"))
}

func TestQueue_Stop(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Start()
	q.Stop()

	select {
	case <-q.Done():
	case <-time.After(time.Second):
		t.Fatal("queue did not stop")
	}
}
