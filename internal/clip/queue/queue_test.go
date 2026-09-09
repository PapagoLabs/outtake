// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package queue

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// datedJob returns the dated job.
//
// Parameters:
//   - id: Identifier.
//   - created: Created.
//
// Returns:
//   - job: The dated job.
func datedJob(id string, created time.Time) *Job {
	job := testJob(id, JobStatusCompleted)

	job.CreatedAt = created

	return job
}

// testJob returns the test job.
//
// Parameters:
//   - id: Identifier.
//   - status: Status.
//
// Returns:
//   - job: The test job.
func testJob(id string, status JobStatus) *Job {
	return &Job{
		ID:            id,
		Type:          JobTypeClip,
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
		Status:        status,
		Progress:      0,
		Error:         "",
		CreatedAt:     time.Time{},
		UpdatedAt:     time.Time{},
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
		ID:            "test-1",
		Type:          JobTypeClip,
		Name:          "",
		Status:        JobStatusPending,
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
		Progress:      0,
		Error:         "",
		CreatedAt:     time.Time{},
		UpdatedAt:     time.Time{},
	}

	q.Submit(job)

	retrieved := q.GetJob("test-1")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-1", retrieved.ID)
}

func TestQueue_GetAllJobs(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	older := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)

	q.Restore(datedJob("old", older))
	q.Restore(datedJob("tie-a", newer))
	q.Restore(datedJob("tie-z", newer))

	want := []string{"tie-z", "tie-a", "old"}

	for range 8 {
		jobs := q.GetAllJobs()
		require.Len(t, jobs, 3)
		assert.Equal(t, want, []string{jobs[0].ID, jobs[1].ID, jobs[2].ID})
	}
}

func TestQueue_ProcessJob_Success(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex

		completed := false

		handler := func(_ context.Context, job *Job) error {
			mu.Lock()
			defer mu.Unlock()

			completed = true
			job.Progress = 50

			return nil
		}

		q := NewQueue(1, handler)
		q.Start()
		t.Cleanup(q.Stop)

		q.Submit(&Job{
			ID:            "success-job",
			Type:          JobTypeClip,
			Name:          "",
			InputPath:     "/tmp/input.mp4",
			Status:        JobStatusPending,
			MediaID:       "",
			MediaTitle:    "",
			MediaType:     "",
			OutputPath:    "",
			StartTime:     0,
			Duration:      0,
			Quality:       "",
			Width:         0,
			FPS:           0,
			AudioIndex:    0,
			CropBlackBars: false,
			Progress:      0,
			Error:         "",
			CreatedAt:     time.Time{},
			UpdatedAt:     time.Time{},
		})

		synctest.Wait()

		job := q.GetJob("success-job")
		require.NotNil(t, job)
		assert.Equal(t, JobStatusCompleted, job.Status)
		assert.Equal(t, 100, job.Progress)

		mu.Lock()
		assert.True(t, completed)
		mu.Unlock()
	})
}

func TestQueue_ProcessJob_Failure(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		handler := func(_ context.Context, _ *Job) error {
			return assert.AnError
		}

		q := NewQueue(1, handler)
		q.Start()
		t.Cleanup(q.Stop)

		q.Submit(&Job{
			ID:            "fail-job",
			Type:          JobTypeGIF,
			Name:          "",
			InputPath:     "/tmp/input.mp4",
			Status:        JobStatusPending,
			MediaID:       "",
			MediaTitle:    "",
			MediaType:     "",
			OutputPath:    "",
			StartTime:     0,
			Duration:      0,
			Quality:       "",
			Width:         0,
			FPS:           0,
			AudioIndex:    0,
			CropBlackBars: false,
			Progress:      0,
			Error:         "",
			CreatedAt:     time.Time{},
			UpdatedAt:     time.Time{},
		})

		synctest.Wait()

		job := q.GetJob("fail-job")
		require.NotNil(t, job)
		assert.Equal(t, JobStatusFailed, job.Status)
		assert.NotEmpty(t, job.Error)
	})
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

func TestQueue_CancelProcessing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		handler := func(ctx context.Context, _ *Job) error {
			close(started)
			<-ctx.Done()

			return ctx.Err()
		}

		q := NewQueue(1, handler)
		q.Start()
		t.Cleanup(q.Stop)

		q.Submit(testJob("cancel-me", JobStatusPending))
		<-started

		assert.True(t, q.Cancel("cancel-me"))
		synctest.Wait()

		job := q.GetJob("cancel-me")
		require.NotNil(t, job)
		assert.Equal(t, JobStatusCancelled, job.Status)
		assert.Equal(t, "canceled", job.Error)
	})
}

func TestQueue_Stop(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		q := NewQueue(1, nil)
		q.Start()
		q.Stop()

		select {
		case <-q.Done():
		default:
			t.Fatal("queue did not stop")
		}
	})
}
