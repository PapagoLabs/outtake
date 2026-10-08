// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package queue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/logging"
)

var errRenderAborted = errors.New("context canceled")

func datedJob(id string, created time.Time) *clip.Job {
	job := testJob(id, clip.StatusCompleted)

	job.CreatedAt = created

	return job
}

func testJob(id string, status clip.Status) *clip.Job {
	return &clip.Job{
		ID:         id,
		Type:       clip.TypeClip,
		Name:       "",
		MediaID:    "",
		MediaTitle: "",
		MediaType:  "",

		StartTime:     0,
		Duration:      0,
		Quality:       "",
		Width:         0,
		FPS:           0,
		AudioIndex:    0,
		CropBlackBars: false,

		CreatedAt: time.Time{},
		UpdatedAt: time.Time{}, InputPath: "",
		OutputPath: "",

		Status:   status,
		Progress: 0,
		Error:    "",
	}
}

func TestNewQueue(t *testing.T) {
	t.Parallel()

	q := NewQueue(2, nil)
	assert.Equal(t, 2, q.workers)
	assert.Empty(t, q.line)
}

func TestQueue_SubmitAndRetrieve(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	job := &clip.Job{
		ID:   "test-1",
		Type: clip.TypeClip,
		Name: "",

		MediaID:    "",
		MediaTitle: "",
		MediaType:  "",

		StartTime:     0,
		Duration:      0,
		Quality:       "",
		Width:         0,
		FPS:           0,
		AudioIndex:    0,
		CropBlackBars: false,

		CreatedAt: time.Time{},
		UpdatedAt: time.Time{}, Status: clip.StatusPending,

		InputPath:  "",
		OutputPath: "",

		Progress: 0,
		Error:    "",
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

		handler := func(_ context.Context, job *clip.Job) error {
			mu.Lock()
			defer mu.Unlock()

			completed = true
			job.Progress = 50

			return nil
		}

		q := NewQueue(1, handler)
		q.Start(t.Context())
		t.Cleanup(q.Stop)

		q.Submit(&clip.Job{
			ID:   "success-job",
			Type: clip.TypeClip,
			Name: "",

			MediaID:    "",
			MediaTitle: "",
			MediaType:  "",

			StartTime:     0,
			Duration:      0,
			Quality:       "",
			Width:         0,
			FPS:           0,
			AudioIndex:    0,
			CropBlackBars: false,

			CreatedAt: time.Time{},
			UpdatedAt: time.Time{}, InputPath: "/tmp/input.mp4",
			Status: clip.StatusPending,

			OutputPath: "",

			Progress: 0,
			Error:    "",
		})

		synctest.Wait()

		job := q.GetJob("success-job")
		require.NotNil(t, job)
		assert.Equal(t, clip.StatusCompleted, job.Status)
		assert.Equal(t, 100, job.Progress)

		mu.Lock()
		assert.True(t, completed)
		mu.Unlock()
	})
}

// TestQueue_ACompletedRenderAdvancesUpdatedAt covers the stamp a finished
// render leaves: clip cards version their file URL by UpdatedAt, so each
// render must move it forward or a browser keeps playing the previous file.
func TestQueue_ACompletedRenderAdvancesUpdatedAt(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		handler := func(_ context.Context, _ *clip.Job) error {
			time.Sleep(time.Second)

			return nil
		}

		q := NewQueue(1, handler)
		q.Start(t.Context())
		t.Cleanup(q.Stop)

		submitted := time.Now()

		q.Submit(&clip.Job{
			ID: "stamped-job", Type: clip.TypeClip, Name: "",
			MediaID: "", MediaTitle: "", MediaType: "",
			StartTime: 0, Duration: 0, Quality: "", Width: 0, FPS: 0,
			AudioIndex: 0, CropBlackBars: false,
			CreatedAt: submitted, UpdatedAt: submitted, InputPath: "/tmp/input.mp4",
			Status: clip.StatusPending, OutputPath: "", Progress: 0, Error: "",
		})

		time.Sleep(2 * time.Second)
		synctest.Wait()

		job := q.GetJob("stamped-job")
		require.NotNil(t, job)
		require.Equal(t, clip.StatusCompleted, job.Status)
		assert.True(t, job.UpdatedAt.After(submitted),
			"the finished render is stamped after the job was submitted")
	})
}

func TestQueue_ProcessJob_Failure(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		handler := func(_ context.Context, _ *clip.Job) error {
			return assert.AnError
		}

		q := NewQueue(1, handler)
		q.Start(t.Context())
		t.Cleanup(q.Stop)

		q.Submit(&clip.Job{
			ID:   "fail-job",
			Type: clip.TypeGIF,
			Name: "",

			MediaID:    "",
			MediaTitle: "",
			MediaType:  "",

			StartTime:     0,
			Duration:      0,
			Quality:       "",
			Width:         0,
			FPS:           0,
			AudioIndex:    0,
			CropBlackBars: false,

			CreatedAt: time.Time{},
			UpdatedAt: time.Time{}, InputPath: "/tmp/input.mp4",
			Status: clip.StatusPending,

			OutputPath: "",

			Progress: 0,
			Error:    "",
		})

		synctest.Wait()

		job := q.GetJob("fail-job")
		require.NotNil(t, job)
		assert.Equal(t, clip.StatusFailed, job.Status)
		assert.NotEmpty(t, job.Error)
	})
}

func TestQueue_DeleteAndRestore(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Restore(testJob("kept", clip.StatusCompleted))
	q.Restore(testJob("gone", clip.StatusCompleted))
	q.Delete("gone")

	assert.NotNil(t, q.GetJob("kept"))
	assert.Nil(t, q.GetJob("gone"))
}

func TestQueue_CancelProcessing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		handler := func(ctx context.Context, _ *clip.Job) error {
			close(started)
			<-ctx.Done()

			return ctx.Err()
		}

		q := NewQueue(1, handler)
		q.Start(t.Context())
		t.Cleanup(q.Stop)

		q.Submit(testJob("cancel-me", clip.StatusPending))
		<-started

		assert.True(t, q.Cancel("cancel-me"))
		synctest.Wait()

		job := q.GetJob("cancel-me")
		require.NotNil(t, job)
		assert.Equal(t, clip.StatusCancelled, job.Status)
		assert.Equal(t, canceledMessage, job.Error)
	})
}

func TestQueue_Stop(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		q := NewQueue(1, nil)
		q.Start(t.Context())
		q.Stop()

		select {
		case <-q.Done():
		default:
			t.Fatal("queue did not stop")
		}
	})
}

func TestQueue_DeleteDuringProcessingSuppressesEveryWrite(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex

		started := make(chan struct{})
		release := make(chan struct{})

		handler := func(_ context.Context, _ *clip.Job) error {
			close(started)
			<-release

			return nil
		}

		var notified []string

		q := NewQueue(1, handler)
		q.SetStatusFunc(func(job *clip.Job) {
			mu.Lock()
			defer mu.Unlock()

			notified = append(notified, job.ID)
		})
		q.Start(t.Context())
		t.Cleanup(q.Stop)

		q.Submit(testJob("deleted-mid-render", clip.StatusPending))

		<-started

		mu.Lock()

		beforeDelete := len(notified)
		mu.Unlock()

		q.Delete("deleted-mid-render")

		close(release)

		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()

		assert.Len(t, notified, beforeDelete,
			"a deleted job must not be reported again, or the status callback writes the row back")
		assert.Empty(t, q.deleted, "the worker clears the tombstone when it finishes")
	})
}

func TestQueue_DeleteBlocksUntilTheCallbackFinishes(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	deleted := make(chan struct{})

	q := NewQueue(1, nil)
	q.Restore(testJob("in-callback", clip.StatusCompleted))
	q.SetStatusFunc(func(_ *clip.Job) {
		close(entered)
		<-release
	})

	go func() {
		q.notify("in-callback")
	}()

	<-entered

	go func() {
		q.Delete("in-callback")
		close(deleted)
	}()

	select {
	case <-deleted:
		t.Fatal("delete ran while the callback was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-deleted
}

func TestQueue_AReportBlocksOnlyADeleteOfItsOwnJob(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})

	q := NewQueue(1, nil)
	q.Restore(testJob("reporting", clip.StatusCompleted))
	q.Restore(testJob("other", clip.StatusCompleted))
	q.SetStatusFunc(func(_ *clip.Job) {
		close(entered)
		<-release
	})

	go q.notify("reporting")

	<-entered

	assert.NotNil(t, q.GetJob("reporting"), "a read does not wait on the report")
	assert.Len(t, q.GetAllJobs(), 2)

	q.Delete("other")
	assert.Nil(t, q.GetJob("other"), "and neither does a delete of another job")

	close(release)
}

func TestQueue_NotifySkipsAJobDeletedFirst(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var persisted []string

	q := NewQueue(1, nil)
	q.SetStatusFunc(func(job *clip.Job) {
		mu.Lock()
		defer mu.Unlock()

		persisted = append(persisted, job.ID)
	})

	q.Submit(testJob("gone-first", clip.StatusProcessing))

	mu.Lock()

	afterSubmit := len(persisted)
	mu.Unlock()

	q.Delete("gone-first")
	q.notify("gone-first")

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, persisted, afterSubmit,
		"a job deleted before the notify must not be persisted again")
}

func TestQueue_DeleteTombstonesOnlyWorkableJobs(t *testing.T) {
	t.Parallel()

	t.Run("a waiting job leaves the line instead", func(t *testing.T) {
		t.Parallel()

		q := NewQueue(1, nil)
		q.Submit(testJob("pending", clip.StatusPending))

		q.Delete("pending")

		assert.Empty(t, q.deleted, "no worker can reach a job that left the line")
		assert.Empty(t, q.line)
		assert.Empty(t, q.heldReason("pending"), "and its id is free again")
	})

	t.Run("a finished job is not", func(t *testing.T) {
		t.Parallel()

		q := NewQueue(1, nil)
		q.Submit(testJob("finished", clip.StatusCompleted))

		q.Delete("finished")

		assert.Empty(t, q.deleted,
			"no worker will ever clear a marker for a job that already finished")
	})

	t.Run("an unknown job is not", func(t *testing.T) {
		t.Parallel()

		q := NewQueue(1, nil)
		q.Delete("never-queued")

		assert.Empty(t, q.deleted)
	})
}

func TestQueue_DeleteStopsACancelledJobStillWaitingInTheChannel(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex

		ran := false

		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			mu.Lock()
			defer mu.Unlock()

			ran = true

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		q.Submit(testJob("canceled-then-deleted", clip.StatusPending))
		q.Cancel("canceled-then-deleted")
		q.Delete("canceled-then-deleted")

		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()

		assert.False(t, ran, "a job deleted while still queued must not run")
		assert.Empty(t, q.dropped, "the canceled marker is cleared by the delete")
		assert.Empty(t, q.deleted, "the worker clears the tombstone it returned on")
	})
}

func TestQueue_DeleteDoesNotTombstoneAJobNoWorkerWillReach(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status clip.Status
	}{
		{name: "a restored canceled job", status: clip.StatusCancelled},
		{name: "a restored completed job", status: clip.StatusCompleted},
		{name: "a restored failed job", status: clip.StatusFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			q := NewQueue(1, nil)
			q.Restore(testJob("restored", test.status))

			for range 50 {
				q.Delete("restored")
			}

			assert.Empty(t, q.deleted,
				"no worker will ever clear a marker for a job that was never enqueued")
		})
	}
}

func TestQueue_DeleteTombstonesAJobAWorkerCanStillReach(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			close(started)
			<-release

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		q.Submit(testJob("unwinding", clip.StatusPending))

		<-started
		q.Cancel("unwinding")
		q.Delete("unwinding")

		assert.Contains(t, q.deleted, "unwinding")

		close(release)
		synctest.Wait()
	})
}

func TestQueue_ReinstateTakesBackAJobAFailedDeleteLeftBehind(t *testing.T) {
	t.Parallel()

	t.Run("a job that was waiting is owned again, and stays stopped", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			var mu sync.Mutex

			ran := false
			blockerStarted := make(chan struct{})
			releaseBlocker := make(chan struct{})

			q := NewQueue(1, func(_ context.Context, job *clip.Job) error {
				if job.ID == "blocker" {
					close(blockerStarted)
					<-releaseBlocker

					return nil
				}

				mu.Lock()
				defer mu.Unlock()

				ran = true

				return nil
			})
			q.Start(t.Context())

			t.Cleanup(q.Stop)

			q.Submit(testJob("blocker", clip.StatusPending))
			<-blockerStarted

			abandoned := testJob("abandoned", clip.StatusPending)
			q.Submit(abandoned)

			q.Delete("abandoned")
			require.Nil(t, q.GetJob("abandoned"), "the delete took the job out of the map")
			require.Empty(t, q.line, "and out of the line")

			q.Reinstate(abandoned)

			reinstated := q.GetJob("abandoned")
			require.NotNil(t, reinstated, "the row that survived the failed delete is owned again")
			assert.Equal(t, clip.StatusCancelled, reinstated.Status)

			close(releaseBlocker)
			synctest.Wait()

			mu.Lock()
			defer mu.Unlock()

			assert.False(t, ran, "a job the delete stopped must not start afterwards")
			assert.Empty(t, q.dropped)
			assert.Equal(t, clip.StatusCancelled, q.GetJob("abandoned").Status,
				"and the job is still owned, since its row survived")
		})
	})

	t.Run("it comes back canceled so the next start does not resubmit", func(t *testing.T) {
		t.Parallel()

		q := NewQueue(1, nil)
		q.Submit(testJob("not-resumed", clip.StatusPending))

		job := q.GetJob("not-resumed")

		q.Delete("not-resumed")
		q.Reinstate(job)

		restored := q.GetJob("not-resumed")
		require.NotNil(t, restored)
		assert.Equal(t, clip.StatusCancelled, restored.Status,
			"a pending render the delete stopped must not be picked up again")
		assert.Equal(t, canceledMessage, restored.Error)
	})
}

func TestQueue_AReinstatedRunningJobSettlesCanceledEvenIfItSucceeds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			close(started)
			<-release

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		q.Submit(testJob("reinstated", clip.StatusPending))

		<-started

		job := q.GetJob("reinstated")
		q.Delete("reinstated")
		q.Reinstate(job)

		close(release)
		synctest.Wait()

		settled := q.GetJob("reinstated")
		require.NotNil(t, settled)
		assert.Equal(t, clip.StatusCancelled, settled.Status,
			"the delete canceled the render, whatever the worker made of it")
		assert.Equal(t, canceledMessage, settled.Error)
	})
}

func TestQueue_AReinstatedRunningJobSettlesCanceledRatherThanFailed(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			close(started)
			<-release

			return errRenderAborted
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		q.Submit(testJob("unwinding-reinstate", clip.StatusPending))

		<-started

		job := q.GetJob("unwinding-reinstate")

		q.Delete("unwinding-reinstate")
		q.Reinstate(job)

		assert.Contains(t, q.dropped, "unwinding-reinstate",
			"the delete is a cancellation the worker must record")
		assert.Equal(t, clip.StatusCancelled, q.GetJob("unwinding-reinstate").Status,
			"and the reinstated clip shows it while the worker unwinds")

		close(release)
		synctest.Wait()

		settled := q.GetJob("unwinding-reinstate")
		require.NotNil(t, settled)
		assert.Equal(t, clip.StatusCancelled, settled.Status,
			"the abort the delete caused is not reported as a failure")
		assert.Equal(t, canceledMessage, settled.Error)
		assert.Empty(t, q.dropped, "the worker consumed the marker")
	})
}

func TestQueue_WorkerSurvivesAPanickingHandler(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		panicked := clip.StatusPending

		q := NewQueue(1, func(_ context.Context, job *clip.Job) error {
			if job.ID == "boom" {
				panic("handler exploded")
			}

			panicked = clip.StatusCompleted

			return nil
		})
		q.SetStatusFunc(func(*clip.Job) {})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		q.Submit(testJob("boom", clip.StatusPending))
		q.Submit(testJob("after", clip.StatusPending))

		synctest.Wait()

		failed := q.GetJob("boom")
		require.NotNil(t, failed)
		assert.Equal(t, clip.StatusFailed, failed.Status)
		assert.Equal(t, ErrJobPanicked.Error(), failed.Error,
			"the job reports a panic rather than the value it was handed")

		assert.Equal(t, clip.StatusCompleted, panicked,
			"the worker carried on and ran the next job, which is what recovering is for")
	})
}

func TestQueue_PanicNilIsStillRecovered(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			//nolint:govet // nilness: panicking with nil is the case under test.
			panic(nil)
		})
		q.SetStatusFunc(func(*clip.Job) {})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		q.Submit(testJob("nil-panic", clip.StatusPending))

		synctest.Wait()

		failed := q.GetJob("nil-panic")
		require.NotNil(t, failed)
		assert.Equal(t, clip.StatusFailed, failed.Status)
	})
}

//nolint:paralleltest // swaps the package-level logger, which is shared state.
func TestQueue_PanicLogCarriesTheStack(t *testing.T) {
	original := logging.Logger

	t.Cleanup(func() { logging.Logger = original })

	var out bytes.Buffer

	logging.Logger = zerolog.New(&out)

	q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
		panic("handler exploded")
	})
	q.SetStatusFunc(func(*clip.Job) {})

	q.runHandler(t.Context(), &clip.Job{ID: "boom", Type: clip.TypeClip})

	entry := out.String()

	assert.Contains(t, entry, `"panic":"handler exploded"`, "the value is reported")
	assert.Contains(t, entry, `"stack":"goroutine `, "the trace is reported, not dropped")
	assert.Contains(t, entry, "queue_test.go",
		"and it names the frame that panicked, so the trace is this job's")
}

func TestQueue_SubmitRefusesASecondJobForAnActiveID(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		renders := 0
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, job *clip.Job) error {
			if job.ID != "once" {
				return nil
			}

			renders++

			select {
			case <-started:
			default:
				close(started)
			}

			<-release

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		require.NoError(t, q.Submit(testJob("once", clip.StatusPending)))

		<-started

		err := q.Submit(testJob("once", clip.StatusPending))
		require.Error(t, err, "a second job for a rendering id is refused")
		require.ErrorIs(t, err, ErrJobActive)

		close(release)
		synctest.Wait()

		assert.Equal(t, 1, renders, "the job ran once, not twice")
	})
}

func TestQueue_RequeueTakesAnIdleJobAgain(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		renders := 0
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			renders++

			close(started)
			<-release

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		job := testJob("idle", clip.StatusCancelled)
		q.Restore(job)

		require.NoError(t, q.Requeue(job),
			"a canceled clip is idle, not active, so it can be run again")

		<-started
		close(release)
		synctest.Wait()

		assert.Equal(t, 1, renders, "the clip ran")
	})
}

func TestQueue_RequeueRefusesWhileAWorkerHoldsTheJob(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			close(started)
			<-release

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		job := testJob("running", clip.StatusPending)
		require.NoError(t, q.Submit(job))

		<-started

		err := q.Requeue(job)
		require.Error(t, err, "a rendering clip is not re-runnable")
		require.ErrorIs(t, err, ErrJobActive)
		assert.Equal(t, clip.StatusProcessing, q.GetJob(job.ID).Status,
			"and the status is left as it was, or the clip would look queued while it renders")

		close(release)
		synctest.Wait()

		assert.Empty(t, q.waiting, "and nothing is left queued behind it")
	})
}

func TestQueue_RequeueIsIdempotentWhileQueued(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		blocking := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, job *clip.Job) error {
			if job.ID == "blocker" {
				close(blocking)
			}

			<-release

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		require.NoError(t, q.Submit(testJob("blocker", clip.StatusPending)))
		<-blocking

		job := testJob("waiting", clip.StatusPending)
		require.NoError(t, q.Requeue(job))

		err := q.Requeue(job)
		require.ErrorIs(t, err, ErrJobActive,
			"a job already in the line would be handed to a second worker")

		close(release)
		synctest.Wait()
	})
}

func TestQueue_RequeueResetsTheJob(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	job := &clip.Job{
		ID:   "reset",
		Type: clip.TypeClip, Status: clip.StatusCancelled,
		Progress: 40,
		Error:    "canceled",
	}
	q.Restore(job)

	require.NoError(t, q.Requeue(job))

	assert.Equal(t, clip.StatusPending, job.Status, "queued for another attempt")
	assert.Equal(
		t,
		clip.StatusPending,
		q.GetJob(job.ID).Status,
		"in the caller's copy and the queue's",
	)
	assert.Equal(t, 0, job.Progress, "and progress starts over, not from where it stopped")
	assert.Empty(t, job.Error, "with no leftover from the canceled attempt")
	assert.NotEmpty(t, q.heldReason(job.ID), "and the id is queued again")
}

func TestQueue_WaitingIsClearedWithTheWorkerRegistration(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			close(entered)

			<-release

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		job := testJob("held", clip.StatusPending)
		require.NoError(t, q.Submit(job))

		<-entered

		q.mu.RLock()

		_, waiting := q.waiting[job.ID]
		_, running := q.cancels[job.ID]

		q.mu.RUnlock()

		assert.False(t, waiting, "a worker holds the entry, so the id is not waiting")
		assert.True(t, running, "and it is registered, or the id was recorded by nothing")

		close(release)
		synctest.Wait()
	})
}

func TestGetJobHandsBackACopy(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Restore(testJob("copy", clip.StatusPending))

	read := q.GetJob("copy")
	require.NotNil(t, read)

	read.Status = clip.StatusFailed
	read.Progress = 99

	after := q.GetJob("copy")
	assert.Equal(t, clip.StatusPending, after.Status,
		"writing to what GetJob returned must not reach the queue's entry")
	assert.Equal(t, 0, after.Progress)
}

func TestGetAllJobsHandsBackCopies(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Restore(testJob("copy", clip.StatusPending))

	jobs := q.GetAllJobs()
	require.Len(t, jobs, 1)

	jobs[0].Status = clip.StatusFailed

	assert.Equal(t, clip.StatusPending, q.GetJob("copy").Status,
		"the listing handed out a copy, not the entry")
}

func TestSetProgressWritesUnderTheLock(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Restore(testJob("copy", clip.StatusProcessing))

	updated := q.SetProgress("copy", 40)
	require.NotNil(t, updated)
	assert.Equal(t, 40, updated.Progress, "the copy carries what was set")
	assert.Equal(t, 40, q.GetJob("copy").Progress, "and so does the entry")

	assert.Nil(t, q.SetProgress("never-queued", 10),
		"a job the queue does not have has nowhere to record progress")
}

func TestSettleNotifiesTheEntryItRecordedAgainst(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		var mu sync.Mutex

		notified := make(map[string]clip.Status)

		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			close(started)
			<-release

			return nil
		})
		q.SetStatusFunc(func(job *clip.Job) {
			mu.Lock()
			defer mu.Unlock()

			notified[job.ID] = job.Status
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		require.NoError(t, q.Submit(testJob("notify", clip.StatusPending)))

		<-started

		q.Delete("notify")
		q.Reinstate(testJob("notify", clip.StatusCompleted))

		close(release)
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()

		assert.Equal(t, clip.StatusCancelled, notified["notify"],
			"the notification carries the settled status, not the captured one")
		assert.Equal(t, clip.StatusCancelled, q.GetJob("notify").Status)
	})
}

func TestStopIsIdempotent(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Start(t.Context())

	q.Stop()
	q.Stop()
}

func TestStopCancelsARunningJob(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		var mu sync.Mutex

		var seen error

		q := NewQueue(1, func(ctx context.Context, _ *clip.Job) error {
			close(started)

			<-ctx.Done()

			mu.Lock()
			defer mu.Unlock()

			seen = ctx.Err()

			return ctx.Err()
		})
		q.Start(t.Context())

		require.NoError(t, q.Submit(testJob("stop", clip.StatusPending)))

		<-started

		q.Stop()

		mu.Lock()
		defer mu.Unlock()

		require.ErrorIs(t, seen, context.Canceled,
			"the job's context follows the queue's, so stopping the queue reaches it")
	})
}

func TestCancelingTheContextStopsTheQueue(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())

		q := NewQueue(1, nil)
		q.Start(ctx)

		t.Cleanup(q.Stop)

		shut := make(chan struct{})

		go func() {
			defer close(shut)

			<-q.Done()
		}()

		cancel()

		select {
		case <-shut:
		case <-time.After(2 * time.Second):
			t.Fatal("canceling the parent did not stop the queue on its own")
		}

		err := q.Submit(testJob("stop", clip.StatusPending))
		require.ErrorIs(t, err, ErrQueueStopped,
			"and a queue canceled out from under its owner refuses further work")

		q.mu.RLock()
		defer q.mu.RUnlock()

		assert.True(t, q.stopped, "the queue is marked stopped, so a submit can refuse")
	})
}

func TestSubmitToAStoppedQueueIsRefused(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Start(t.Context())

	q.Stop()

	err := q.Submit(testJob("stop", clip.StatusPending))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrQueueStopped)

	q.mu.RLock()
	defer q.mu.RUnlock()

	assert.NotContains(t, q.waiting, "stop",
		"a refused submit leaves no mark, since nothing will reach it")
}

func TestSubmitNeverWaitsOnAWorkerAndJobsRunInOrder(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const backlog = 1000

		release := make(chan struct{})
		holding := make(chan struct{})

		var mu sync.Mutex

		var order []string

		q := NewQueue(1, func(_ context.Context, job *clip.Job) error {
			if job.ID == "blocker" {
				close(holding)
				<-release

				return nil
			}

			mu.Lock()
			defer mu.Unlock()

			order = append(order, job.ID)

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		require.NoError(t, q.Submit(testJob("blocker", clip.StatusPending)))
		<-holding

		want := make([]string, 0, backlog)

		for i := range backlog {
			id := fmt.Sprintf("job-%04d", i)

			want = append(want, id)

			require.NoError(t, q.Submit(testJob(id, clip.StatusPending)),
				"a submit is accepted while the only worker is busy")
		}

		close(release)
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()

		assert.Equal(t, want, order, "jobs run in the order they were submitted")
	})
}

func TestEveryIdleWorkerIsWoken(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})

		var running sync.WaitGroup

		running.Add(2)

		q := NewQueue(2, func(_ context.Context, _ *clip.Job) error {
			running.Done()
			<-release

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		require.NoError(t, q.Submit(testJob("first", clip.StatusPending)))
		require.NoError(t, q.Submit(testJob("second", clip.StatusPending)))

		// Both jobs start at once, or the second waits on the first and this
		// never returns.
		running.Wait()

		close(release)
		synctest.Wait()
	})
}

func TestSubmitKeepsItsOwnCopy(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		q := NewQueue(1, func(_ context.Context, job *clip.Job) error {
			job.Name = "renamed by the handler"

			return nil
		})

		job := testJob("copied", clip.StatusPending)
		require.NoError(t, q.Submit(job))

		job.Name = "renamed by the caller"
		job.Status = clip.StatusFailed

		queued := q.GetJob("copied")
		assert.Empty(t, queued.Name, "a write to the submitted job does not reach the queue")
		assert.Equal(t, clip.StatusPending, queued.Status)

		q.Start(t.Context())
		t.Cleanup(q.Stop)
		synctest.Wait()

		settled := q.GetJob("copied")
		assert.Empty(t, settled.Name, "the handler renders a snapshot, not the entry")
		assert.Equal(t, clip.StatusCompleted, settled.Status)
		assert.Equal(
			t,
			clip.StatusFailed,
			job.Status,
			"and the queue never writes to the caller's job",
		)
	})
}

func TestCancelThenRequeueRendersOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		holding := make(chan struct{})

		var mu sync.Mutex

		renders := 0

		q := NewQueue(1, func(_ context.Context, job *clip.Job) error {
			if job.ID == "blocker" {
				close(holding)
				<-release

				return nil
			}

			mu.Lock()
			defer mu.Unlock()

			renders++

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		require.NoError(t, q.Submit(testJob("blocker", clip.StatusPending)))
		<-holding

		job := testJob("again", clip.StatusPending)
		require.NoError(t, q.Submit(job))
		require.True(t, q.Cancel("again"))

		require.NoError(
			t,
			q.Requeue(job),
			"a canceled job that never started is free to queue again",
		)

		close(release)
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()

		assert.Equal(t, 1, renders, "the canceled attempt left nothing behind to run")
		assert.Equal(t, clip.StatusCompleted, q.GetJob("again").Status)
	})
}

func TestEveryReportCarriesTheLatestState(t *testing.T) {
	t.Parallel()

	var reported []clip.Status

	q := NewQueue(1, nil)
	q.Restore(testJob("racing", clip.StatusPending))
	q.SetStatusFunc(func(job *clip.Job) {
		reported = append(reported, job.Status)
	})

	require.True(t, q.Cancel("racing"))
	require.NotNil(t, q.SetProgress("racing", 40))

	assert.Equal(t, []clip.Status{clip.StatusCancelled, clip.StatusCancelled}, reported,
		"a progress report after a cancel carries the cancel, so it cannot write it away")
}

func TestAJobInterruptedByShutdownStaysPending(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})

		q := NewQueue(1, func(ctx context.Context, _ *clip.Job) error {
			close(started)

			<-ctx.Done()

			return ctx.Err()
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		require.NoError(t, q.Submit(testJob("stop", clip.StatusPending)))

		<-started

		q.Stop()

		settled := q.GetJob("stop")
		require.NotNil(t, settled)
		assert.Equal(t, clip.StatusPending, settled.Status,
			"an interrupted render is unfinished work, not a failure")
		assert.Empty(t, settled.Error, "and carries no error from being stopped")
	})
}

// editOf returns an edit that leaves a job as it is, for a test to change one
// field of.
//
// Parameters:
//   - job: The job the edit starts from.
//
// Returns:
//   - edit: An edit carrying every field of job.
func editOf(job *clip.Job) clip.Edit {
	return clip.Edit{
		Type:          job.Type,
		Name:          job.Name,
		Quality:       job.Quality,
		Start:         job.StartTime,
		Length:        job.Duration,
		Width:         job.Width,
		FPS:           job.FPS,
		AudioIndex:    job.AudioIndex,
		CropBlackBars: job.CropBlackBars,
		WebSafeColor:  &job.WebSafeColor,
		PreserveHDR:   &job.PreserveHDR,
	}
}

// discardSave is a save that always succeeds.
//
// Returns:
//   - err: Always nil.
func discardSave(*clip.Job) error {
	return nil
}

// TestEditSavesAndUpdatesTheEntry covers an edit of a settled job: the save
// receives the edited job and every later read sees it.
func TestEditSavesAndUpdatesTheEntry(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	job := testJob("settled", clip.StatusCompleted)

	job.Name = "Before"
	q.Restore(job)

	edit := editOf(job)

	edit.Name = "After"

	var saved *clip.Job

	edited, rerender, err := q.Edit(
		"settled",
		edit,
		"/out/settled.mp4",
		func(snapshot *clip.Job) error {
			saved = snapshot

			return nil
		},
	)
	require.NoError(t, err)

	assert.False(t, rerender, "a rename leaves the file as it is")
	assert.Equal(t, "After", edited.Name)
	require.NotNil(t, saved)
	assert.Equal(t, "After", saved.Name, "the save receives the edited job")
	assert.Equal(t, "After", q.GetJob("settled").Name, "and so does every later read")
	assert.Equal(t, clip.StatusCompleted, q.GetJob("settled").Status, "the status is left alone")
}

// TestEditRefusesAJobTheQueueDoesNotHave covers an unknown id.
func TestEditRefusesAJobTheQueueDoesNotHave(t *testing.T) {
	t.Parallel()

	_, _, err := NewQueue(1, nil).Edit("absent", clip.Edit{}, "", discardSave)

	require.ErrorIs(t, err, ErrJobNotFound)
}

// TestEditOfARenderingJob covers the edits a running render may take: a
// change that leaves the file the same, and nothing when a render is asked
// for.
func TestEditOfARenderingJob(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *clip.Job) error {
			close(started)
			<-release

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		job := testJob("rendering", clip.StatusPending)
		require.NoError(t, q.Submit(job))

		<-started

		moved := editOf(job)

		moved.Start = time.Minute

		_, _, err := q.Edit("rendering", moved, "", discardSave)
		require.ErrorIs(t, err, ErrJobActive, "a new window would make the file stale")
		assert.Zero(t, q.GetJob("rendering").StartTime, "and the refused edit changes nothing")

		renamed := editOf(job)

		renamed.Name = "Renamed"

		_, _, err = q.EditAndRegenerate("rendering", renamed, "", discardSave)
		require.ErrorIs(t, err, ErrJobActive, "a render cannot be queued while one runs")

		_, rerender, err := q.Edit("rendering", renamed, "", discardSave)
		require.NoError(t, err, "a rename leaves the file as it is")
		assert.False(t, rerender)

		close(release)
		synctest.Wait()

		settled := q.GetJob("rendering")
		assert.Equal(t, clip.StatusCompleted, settled.Status)
		assert.Equal(
			t,
			"Renamed",
			settled.Name,
			"the worker settles the entry, so the rename survives",
		)
	})
}

// TestEditOfAWaitingJobIsRendered covers a job still in the line: the worker
// that takes it renders the edited values, so it is not queued twice.
func TestEditOfAWaitingJobIsRendered(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		holding := make(chan struct{})
		release := make(chan struct{})

		var mu sync.Mutex

		var rendered time.Duration

		q := NewQueue(1, func(_ context.Context, job *clip.Job) error {
			if job.ID == "blocker" {
				close(holding)
				<-release

				return nil
			}

			mu.Lock()
			defer mu.Unlock()

			rendered = job.StartTime

			return nil
		})
		q.Start(t.Context())

		t.Cleanup(q.Stop)

		require.NoError(t, q.Submit(testJob("blocker", clip.StatusPending)))
		<-holding

		job := testJob("waiting", clip.StatusPending)
		require.NoError(t, q.Submit(job))

		moved := editOf(job)

		moved.Start = time.Minute

		_, rerender, err := q.EditAndRegenerate("waiting", moved, "", discardSave)
		require.NoError(t, err)
		assert.False(t, rerender, "the job is already queued")

		close(release)
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()

		assert.Equal(t, time.Minute, rendered, "the worker rendered the edited window")
	})
}

// TestEditOfTheTypeMovesTheOutput covers a type change on a settled job: the
// job renders to the new type's file and has to be queued again.
func TestEditOfTheTypeMovesTheOutput(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	job := testJob("retyped", clip.StatusCompleted)

	job.OutputPath = "/out/retyped.mp4"
	q.Restore(job)

	edit := editOf(job)

	edit.Type = clip.TypeGIF

	var saved *clip.Job

	edited, queued, err := q.Edit("retyped", edit, "/out/retyped.gif", func(job *clip.Job) error {
		saved = job

		return nil
	})
	require.NoError(t, err)

	assert.True(t, queued, "a file of the old type does not match the clip any more")
	assert.Equal(t, "/out/retyped.gif", edited.OutputPath)
	assert.Equal(t, clip.TypeGIF, q.GetJob("retyped").Type)
	assert.Equal(t, clip.StatusPending, edited.Status)
	assert.Equal(t, clip.StatusPending, saved.Status,
		"the save records the clip as queued, in the same step as the edit")
	assert.Equal(t, []string{"retyped"}, q.line, "and it is in the line once the save succeeded")
}

// TestAFailedEditSaveLeavesTheJobAsItWas covers a save the store refuses.
func TestAFailedEditSaveLeavesTheJobAsItWas(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	job := testJob("unsaved", clip.StatusCompleted)

	job.Name = "Before"
	job.OutputPath = "/out/unsaved.mp4"
	q.Restore(job)

	edit := editOf(job)

	edit.Name = "After"
	edit.Type = clip.TypeGIF

	_, _, err := q.Edit("unsaved", edit, "/out/unsaved.gif", func(*clip.Job) error {
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)

	kept := q.GetJob("unsaved")
	assert.Equal(t, "Before", kept.Name, "the queue does not show an edit the store refused")
	assert.Equal(t, clip.TypeClip, kept.Type)
	assert.Equal(t, "/out/unsaved.mp4", kept.OutputPath)
}

// TestADeleteWaitsForAnEditSave covers the barrier an edit's save shares with
// status reports, so a delete cannot be followed by a save writing the row
// back.
func TestADeleteWaitsForAnEditSave(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	deleted := make(chan struct{})

	q := NewQueue(1, nil)

	job := testJob("saving", clip.StatusCompleted)
	q.Restore(job)

	go func() {
		_, _, _ = q.Edit("saving", editOf(job), "", func(*clip.Job) error {
			close(entered)
			<-release

			return nil
		})
	}()

	<-entered

	go func() {
		q.Delete("saving")
		close(deleted)
	}()

	select {
	case <-deleted:
		t.Fatal("delete ran while the save was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-deleted
}

// TestAFailedSaveOfAQueuingEditTakesTheJobBackOut covers an edit that would
// have queued the job: a refused save leaves nothing queued and nothing
// changed.
func TestAFailedSaveOfAQueuingEditTakesTheJobBackOut(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	job := testJob("refused", clip.StatusCompleted)

	job.Progress = 100
	job.OutputPath = "/out/refused.mp4"
	q.Restore(job)

	edit := editOf(job)

	edit.Type = clip.TypeGIF

	_, _, err := q.Edit("refused", edit, "/out/refused.gif", func(*clip.Job) error {
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)

	kept := q.GetJob("refused")
	assert.Equal(t, clip.StatusCompleted, kept.Status)
	assert.Equal(t, 100, kept.Progress)
	assert.Equal(t, clip.TypeClip, kept.Type)
	assert.Equal(t, "/out/refused.mp4", kept.OutputPath)
	assert.Empty(t, q.line, "a worker never renders an edit the store refused")
	assert.Empty(t, q.heldReason("refused"), "and the id is free to queue")
}

// TestAStoppedQueueRefusesAQueuingEdit covers an edit that would have to queue
// the job on a queue that can no longer run it.
func TestAStoppedQueueRefusesAQueuingEdit(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Start(t.Context())
	q.Stop()

	job := testJob("stopped", clip.StatusCompleted)
	q.Restore(job)

	edit := editOf(job)

	edit.Type = clip.TypeGIF

	_, _, err := q.Edit("stopped", edit, "/out/stopped.gif", func(*clip.Job) error {
		t.Error("nothing is saved for an edit the queue refused")

		return nil
	})
	require.ErrorIs(t, err, ErrQueueStopped)

	assert.Equal(t, clip.TypeClip, q.GetJob("stopped").Type, "and nothing changed")

	renamed := editOf(job)

	renamed.Name = "Renamed"

	_, _, err = q.Edit("stopped", renamed, "", discardSave)
	require.NoError(t, err, "an edit that queues nothing still works")
}

// TestACancelDuringAQueuingEditStands covers a cancel that lands while the
// edit is being saved: the job does not join the line afterwards.
func TestACancelDuringAQueuingEditStands(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	job := testJob("canceled", clip.StatusCompleted)
	q.Restore(job)

	canceled := make(chan bool, 1)

	_, _, err := q.EditAndRegenerate(
		"canceled",
		editOf(job),
		"/out/canceled.mp4",
		func(*clip.Job) error {
			// The cancel finishes its own write once this save is done, so it runs
			// beside the save and the save waits only for its status change.
			go func() { canceled <- q.Cancel("canceled") }()

			require.Eventually(t, func() bool {
				return q.GetJob("canceled").Status == clip.StatusCancelled
			}, time.Second, time.Millisecond)

			return nil
		},
	)
	require.NoError(t, err)
	require.True(t, <-canceled)

	assert.Equal(t, clip.StatusCancelled, q.GetJob("canceled").Status)
	assert.Empty(t, q.line, "a job canceled before it joined the line stays out of it")
	assert.Empty(t, q.heldReason("canceled"))
}

// TestAdoptKeepsAnEntryTheQueueHolds covers a stored record arriving after
// another caller changed the queue's entry.
func TestAdoptKeepsAnEntryTheQueueHolds(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	edited := testJob("adopted", clip.StatusCompleted)

	edited.Name = "Edited"
	q.Restore(edited)

	stale := testJob("adopted", clip.StatusCompleted)

	stale.Name = "Stored"
	q.Adopt(stale)

	assert.Equal(
		t,
		"Edited",
		q.GetJob("adopted").Name,
		"the stored record does not replace the edit",
	)

	q.Adopt(testJob("new", clip.StatusCompleted))
	assert.NotNil(t, q.GetJob("new"), "a record the queue lacks is taken")
}
