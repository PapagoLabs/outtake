// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package queue

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/logging"
)

// Job ids used by the submit and requeue tests.
const (
	testOnceID    = "once"
	testProbeID   = "probe"
	testBlockerID = "blocker"
	testIdleID    = "idle"
	testHeldID    = "held"
	testCopyID    = "copy"
)

// testPanicJobID is the job id used by the panic tests.
const testPanicJobID = "boom"

// errRenderAborted stands in for the error a handler returns when its context is
// canceled under it.
var errRenderAborted = errors.New("context canceled")

func datedJob(id string, created time.Time) *Job {
	job := testJob(id, JobStatusCompleted)

	job.CreatedAt = created

	return job
}

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

// TestQueue_DeleteDuringProcessingSuppressesEveryWrite covers the tombstone.
//
// Delete removes the row while a worker still holds the pointer. Everything the
// queue does on the way out persists the job, so without the tombstone the row
// Delete just removed is written straight back — a delete that appears to do
// nothing until the next render.
func TestQueue_DeleteDuringProcessingSuppressesEveryWrite(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex

		started := make(chan struct{})
		release := make(chan struct{})

		handler := func(_ context.Context, _ *Job) error {
			close(started)
			<-release

			return nil
		}

		var notified []string

		q := NewQueue(1, handler)
		q.SetStatusFunc(func(job *Job) {
			mu.Lock()
			defer mu.Unlock()

			notified = append(notified, job.ID)
		})
		q.Start()
		t.Cleanup(q.Stop)

		q.Submit(&Job{ID: "deleted-mid-render", Type: JobTypeClip, Status: JobStatusPending})

		<-started

		// The job is now inside the handler, holding the only pointer to it. The
		// "processing" notification it already produced is expected, since the job
		// existed then, so the count is taken here and only a later rise is a
		// resurrection.
		mu.Lock()

		beforeDelete := len(notified)
		mu.Unlock()

		q.Delete("deleted-mid-render")

		close(release)

		// Let the worker run its whole exit path before asserting on it.
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()

		assert.Len(t, notified, beforeDelete,
			"a deleted job must not be reported again, or the status callback writes the row back")
		assert.Empty(t, q.deleted, "the worker clears the tombstone when it finishes")
	})
}

// TestQueue_DeleteBlocksUntilTheCallbackFinishes covers the one step.
//
// The deleted check and the callback are held under a single read lock, so a
// Delete cannot land between them. That is observable as a Delete that waits
// for a callback already in flight rather than racing past it.
func TestQueue_DeleteBlocksUntilTheCallbackFinishes(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	deleted := make(chan struct{})

	q := NewQueue(1, nil)
	q.SetStatusFunc(func(_ *Job) {
		close(entered)
		<-release
	})

	go func() {
		q.notify(&Job{ID: "in-callback", Type: JobTypeClip, Status: JobStatusCompleted})
	}()

	<-entered

	go func() {
		q.Delete("in-callback")
		close(deleted)
	}()

	// The callback is parked, so the lock is still held and Delete cannot be
	// past it. A delete that completed here would be the gap.
	select {
	case <-deleted:
		t.Fatal("delete ran while the callback was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-deleted
}

// TestQueue_NotifySkipsAJobDeletedFirst is the deterministic half: once Delete
// has returned, the tombstone is in place and no later notify persists the job.
func TestQueue_NotifySkipsAJobDeletedFirst(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var persisted []string

	q := NewQueue(1, nil)
	q.SetStatusFunc(func(job *Job) {
		mu.Lock()
		defer mu.Unlock()

		persisted = append(persisted, job.ID)
	})

	q.Submit(&Job{ID: "gone-first", Type: JobTypeClip, Status: JobStatusProcessing})

	// Submit notifies in its own right, so the count is taken after that and
	// only a later rise is the resurrection this guards against.
	mu.Lock()

	afterSubmit := len(persisted)
	mu.Unlock()

	q.Delete("gone-first")
	q.notify(&Job{ID: "gone-first", Type: JobTypeClip, Status: JobStatusCompleted})

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, persisted, afterSubmit,
		"a job deleted before the notify must not be persisted again")
}

// TestQueue_DeleteTombstonesOnlyWorkableJobs keeps the tombstone map bounded.
//
// A finished job has no worker coming to clear a marker, so leaving one behind
// would grow the map for the life of the process.
func TestQueue_DeleteTombstonesOnlyWorkableJobs(t *testing.T) {
	t.Parallel()

	t.Run("a pending job is tombstoned so a worker skips it", func(t *testing.T) {
		t.Parallel()

		q := NewQueue(1, nil)
		q.Submit(&Job{ID: "pending", Type: JobTypeClip, Status: JobStatusPending})

		q.Delete("pending")

		assert.Contains(t, q.deleted, "pending",
			"a job still in the channel can be picked up, so it needs a tombstone")
	})

	t.Run("a finished job is not", func(t *testing.T) {
		t.Parallel()

		q := NewQueue(1, nil)
		q.Submit(&Job{ID: "finished", Type: JobTypeClip, Status: JobStatusCompleted})

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

// TestQueue_DeleteStopsACancelledJobStillWaitingInTheChannel covers the marker
// bookkeeping for a job that was canceled and then deleted before a worker
// picked it up.
//
// The canceled marker is cleared by the delete, so the tombstone is the only
// thing left stopping a job that is still sitting in the channel from running.
// Asserting on which marker exists would not catch a job that runs anyway.
func TestQueue_DeleteStopsACancelledJobStillWaitingInTheChannel(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex

		ran := false

		q := NewQueue(1, func(_ context.Context, _ *Job) error {
			mu.Lock()
			defer mu.Unlock()

			ran = true

			return nil
		})
		q.Start()

		t.Cleanup(q.Stop)

		q.Submit(&Job{ID: "canceled-then-deleted", Type: JobTypeClip, Status: JobStatusPending})
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

// TestQueue_DeleteDoesNotTombstoneAJobNoWorkerWillReach is the containment guard.
//
// A tombstone is only ever cleared by a worker. Restore registers a job without
// enqueueing it, so a restored job has no worker coming: tombstoning it leaves
// the entry in the map for the life of the process, and the next delete returns
// early because the job has left the map. Repeated cancel-and-delete cycles
// would then retain a marker each.
func TestQueue_DeleteDoesNotTombstoneAJobNoWorkerWillReach(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status JobStatus
	}{
		{name: "a restored canceled job", status: JobStatusCancelled},
		{name: "a restored completed job", status: JobStatusCompleted},
		{name: "a restored failed job", status: JobStatusFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			q := NewQueue(1, nil)
			q.Restore(&Job{ID: "restored", Type: JobTypeClip, Status: test.status})

			for range 50 {
				q.Delete("restored")
			}

			assert.Empty(t, q.deleted,
				"no worker will ever clear a marker for a job that was never enqueued")
		})
	}
}

// TestQueue_DeleteTombstonesAJobAWorkerCanStillReach is the other half: the
// guard above must not have gone too far and stopped marking jobs that really
// do have a worker behind them.
func TestQueue_DeleteTombstonesAJobAWorkerCanStillReach(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *Job) error {
			close(started)
			<-release

			return nil
		})
		q.Start()

		t.Cleanup(q.Stop)

		q.Submit(&Job{ID: "unwinding", Type: JobTypeClip, Status: JobStatusPending})

		<-started
		q.Cancel("unwinding")
		q.Delete("unwinding")

		// Canceled while the worker held it, so the status alone no longer says a
		// worker is involved. The live cancel entry is what has to keep it marked,
		// or the worker's own write on the way out goes through.
		assert.Contains(t, q.deleted, "unwinding")

		close(release)
		synctest.Wait()
	})
}

// TestQueue_ReinstateTakesBackAJobAFailedDeleteLeftBehind covers the
// compensation.
//
// Delete takes the job out before the row goes, so a status change cannot write
// the row back in between. If the row removal then fails, the job has to come
// back: left out, the row has nothing owning it, a cancel would not find it,
// and the next start would resubmit a render the delete had already stopped.
func TestQueue_ReinstateTakesBackAJobAFailedDeleteLeftBehind(t *testing.T) {
	t.Parallel()

	t.Run("a job still in the channel is owned again, and stays stopped", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			var mu sync.Mutex

			ran := false
			blockerStarted := make(chan struct{})
			releaseBlocker := make(chan struct{})

			q := NewQueue(1, func(_ context.Context, job *Job) error {
				if job.ID == testBlockerID {
					close(blockerStarted)
					<-releaseBlocker

					return nil
				}

				mu.Lock()
				defer mu.Unlock()

				ran = true

				return nil
			})
			q.Start()

			t.Cleanup(q.Stop)

			// The single worker is held on another job, so this one is known to be
			// sitting in the channel rather than in a worker.
			q.Submit(&Job{ID: testBlockerID, Type: JobTypeClip, Status: JobStatusPending})
			<-blockerStarted

			abandoned := &Job{ID: "abandoned", Type: JobTypeClip, Status: JobStatusPending}
			q.Submit(abandoned)

			q.Delete("abandoned")
			require.Nil(t, q.GetJob("abandoned"), "the delete took the job out of the map")
			require.Contains(t, q.deleted, "abandoned", "a queued job is marked as one to stop")

			q.Reinstate(abandoned)

			// Compared by value, not by identity. The accessors hand back copies, so
			// the queue owns an equal job rather than the caller's object.
			assert.Equal(t, abandoned, q.GetJob("abandoned"),
				"the row that survived the failed delete is owned again")
			assert.Empty(t, q.deleted, "the tombstone is consumed by the reinstate")

			// The job was still in the channel, so nothing else stops it starting
			// now. It has to have become a canceled one rather than run.
			close(releaseBlocker)
			synctest.Wait()

			mu.Lock()
			defer mu.Unlock()

			assert.False(t, ran, "a job the delete stopped must not start afterwards")
			assert.Empty(t, q.dropped,
				"the worker returned on the canceled marker and consumed it, so none is retained")
			assert.Equal(t, abandoned, q.GetJob("abandoned"),
				"and the job is still owned, since its row survived")
		})
	})

	t.Run("it comes back canceled so the next start does not resubmit", func(t *testing.T) {
		t.Parallel()

		q := NewQueue(1, nil)
		q.Submit(&Job{ID: "not-resumed", Type: JobTypeClip, Status: JobStatusPending})

		job := q.GetJob("not-resumed")

		q.Delete("not-resumed")
		q.Reinstate(job)

		restored := q.GetJob("not-resumed")
		require.NotNil(t, restored)
		assert.Equal(t, JobStatusCancelled, restored.Status,
			"a pending render the delete stopped must not be picked up again")
		assert.Equal(t, string(JobStatusCancelled), restored.Error)
	})
}

// TestQueue_SettleClearsTheErrorOnAReinstatedJobThatSucceeds covers the state a
// reinstated job can reach.
//
// Reinstate marks a job canceled while a worker is unwinding, and that worker
// may then report success. Without clearing the error, the job ends up completed
// with an error saying it was canceled, which reads as a contradiction to
// anything showing both.
func TestQueue_SettleClearsTheErrorOnAReinstatedJobThatSucceeds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *Job) error {
			close(started)
			<-release

			return nil
		})
		q.Start()

		t.Cleanup(q.Stop)

		q.Submit(&Job{ID: "reinstated", Type: JobTypeClip, Status: JobStatusPending})

		<-started

		// The delete takes the job out, then the row removal fails and the job
		// is handed back marked canceled. The worker is still unwinding.
		job := q.GetJob("reinstated")
		q.Delete("reinstated")
		q.Reinstate(job)

		close(release)
		synctest.Wait()

		settled := q.GetJob("reinstated")
		require.NotNil(t, settled)
		assert.Equal(t, JobStatusCompleted, settled.Status)
		assert.Empty(t, settled.Error,
			"a completed job must not still be carrying its cancellation error")
	})
}

// TestQueue_ReinstateLeavesAnUnwindingJobToRecordItsOutcome is the other half of
// the queued case.
//
// A worker already holding the job must be left alone. Marking it would make
// that worker discard a real outcome, so the reinstate records the cancellation
// and then the worker's own settle overwrites it with what the render ended as.
func TestQueue_ReinstateLeavesAnUnwindingJobToRecordItsOutcome(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *Job) error {
			close(started)
			<-release

			return errRenderAborted
		})
		q.Start()

		t.Cleanup(q.Stop)

		q.Submit(&Job{ID: "unwinding-reinstate", Type: JobTypeClip, Status: JobStatusPending})

		<-started

		job := q.GetJob("unwinding-reinstate")

		q.Delete("unwinding-reinstate")
		q.Reinstate(job)

		assert.Empty(t, q.dropped,
			"a job a worker is unwinding must not be marked to stop, or its outcome is lost")

		close(release)
		synctest.Wait()

		settled := q.GetJob("unwinding-reinstate")
		require.NotNil(t, settled)
		assert.Equal(t, JobStatusFailed, settled.Status,
			"the worker's own outcome is recorded, not the cancellation the reinstate wrote")
		assert.Equal(t, errRenderAborted.Error(), settled.Error)
	})
}

// TestQueue_WorkerSurvivesAPanickingHandler covers the whole point.
//
// A panic in a worker goroutine takes the process with it. The handler is the
// only part of a worker running code this package does not control, so that is
// where the recovery goes, and the job it loses is recorded as failed rather than
// left showing as processing forever.
func TestQueue_WorkerSurvivesAPanickingHandler(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		panicked := JobStatusPending

		q := NewQueue(1, func(_ context.Context, job *Job) error {
			if job.ID == "boom" {
				panic("handler exploded")
			}

			panicked = JobStatusCompleted

			return nil
		})
		q.SetStatusFunc(func(*Job) {})
		q.Start()

		t.Cleanup(q.Stop)

		q.Submit(&Job{ID: testPanicJobID, Type: JobTypeClip, Status: JobStatusPending})
		q.Submit(&Job{ID: "after", Type: JobTypeClip, Status: JobStatusPending})

		synctest.Wait()

		failed := q.GetJob("boom")
		require.NotNil(t, failed)
		assert.Equal(t, JobStatusFailed, failed.Status)
		assert.Equal(t, ErrJobPanicked.Error(), failed.Error,
			"the job reports a panic rather than the value it was handed")

		assert.Equal(t, JobStatusCompleted, panicked,
			"the worker carried on and ran the next job, which is what recovering is for")
	})
}

// TestQueue_PanicNilIsStillRecovered covers the case recover alone would miss.
//
// A bare panic(nil) used to return nil from recover, which would read as "no
// panic" and leave the job stuck processing. Since Go 1.21 it arrives as a
// PanicNilError instead, so the nil check is safe, and this pins that.
func TestQueue_PanicNilIsStillRecovered(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		q := NewQueue(1, func(_ context.Context, _ *Job) error {
			//nolint:govet // nilness: panicking with nil is the case under test.
			panic(nil)
		})
		q.SetStatusFunc(func(*Job) {})
		q.Start()

		t.Cleanup(q.Stop)

		q.Submit(&Job{ID: "nil-panic", Type: JobTypeClip, Status: JobStatusPending})

		synctest.Wait()

		failed := q.GetJob("nil-panic")
		require.NotNil(t, failed)
		assert.Equal(t, JobStatusFailed, failed.Status)
	})
}

// TestQueue_PanicLogCarriesTheStack guards the one thing that makes the log
// trustworthy when a handler panics.
//
// The zerolog Stack method only renders when an error is attached to the event, and
// this one carries the panic value as a field rather than as an error. Left as
// it was, the trace was silently dropped and the log said a job panicked without
// saying where.
//
//nolint:paralleltest // swaps the package-level logger, which is shared state.
func TestQueue_PanicLogCarriesTheStack(t *testing.T) {
	original := logging.Logger

	t.Cleanup(func() { logging.Logger = original })

	var out bytes.Buffer

	logging.Logger = zerolog.New(&out)

	q := NewQueue(1, func(_ context.Context, _ *Job) error {
		panic("handler exploded")
	})
	q.SetStatusFunc(func(*Job) {})

	q.runHandler(t.Context(), &Job{ID: testPanicJobID, Type: JobTypeClip})

	entry := out.String()

	assert.Contains(t, entry, `"panic":"handler exploded"`, "the value is reported")
	assert.Contains(t, entry, `"stack":"goroutine `, "the trace is reported, not dropped")
	assert.Contains(t, entry, "queue_test.go",
		"and it names the frame that panicked, so the trace is this job's")
}

// TestQueue_SubmitRefusesASecondJobForAnActiveID covers the duplicate.
//
// Two workers on one id render the same output path at once and race on the
// status the first one writes. Re-submitting a clip that is already queued or
// rendering has to be refused, not queued behind it.
func TestQueue_SubmitRefusesASecondJobForAnActiveID(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		renders := 0
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, job *Job) error {
			if job.ID != testOnceID {
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
		q.Start()

		t.Cleanup(q.Stop)

		require.NoError(t, q.Submit(&Job{
			ID: testOnceID, Type: JobTypeClip, Status: JobStatusPending,
		}))

		<-started

		err := q.Submit(&Job{ID: testOnceID, Type: JobTypeClip, Status: JobStatusPending})
		require.Error(t, err, "a second job for a rendering id is refused")
		require.ErrorIs(t, err, ErrJobActive)

		close(release)
		synctest.Wait()

		assert.Equal(t, 1, renders, "the job ran once, not twice")
	})
}

// TestQueue_RequeueTakesAnIdleJobAgain is the case the guard must not refuse.
//
// A clip that was canceled is still in the map, marked canceled, with nothing
// running or queued for it. Re-running it is the whole point of the regenerate
// button, and a check that reads the status would see the Pending the requeue
// itself writes and call it a second job for its own id.
func TestQueue_RequeueTakesAnIdleJobAgain(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		renders := 0
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *Job) error {
			renders++

			close(started)
			<-release

			return nil
		})
		q.Start()

		t.Cleanup(q.Stop)

		job := &Job{ID: testIdleID, Type: JobTypeClip, Status: JobStatusCancelled}
		q.Restore(job)

		require.NoError(t, q.Requeue(job),
			"a canceled clip is idle, not active, so it can be run again")
		// The job's own fields are not asserted here. A worker would be writing
		// them under the queue's lock, and reading them from the test would race
		// with it. TestQueue_RequeueResetsTheJob covers the reset with no worker
		// running to touch it.

		<-started
		close(release)
		synctest.Wait()

		assert.Equal(t, 1, renders, "the clip ran")
	})
}

// TestQueue_RequeueRefusesWhileAWorkerHoldsTheJob is the other direction, and
// the one the status-based check was never able to get right on its own.
func TestQueue_RequeueRefusesWhileAWorkerHoldsTheJob(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *Job) error {
			close(started)
			<-release

			return nil
		})
		q.Start()

		t.Cleanup(q.Stop)

		job := &Job{ID: "running", Type: JobTypeClip, Status: JobStatusPending}
		require.NoError(t, q.Submit(job))

		<-started

		err := q.Requeue(job)
		require.Error(t, err, "a rendering clip is not re-runnable")
		require.ErrorIs(t, err, ErrJobActive)
		assert.Equal(t, JobStatusProcessing, job.Status,
			"and the status is left as it was, or the clip would look queued while it renders")

		close(release)
		synctest.Wait()

		assert.Empty(t, q.waiting, "and nothing is left queued behind it")
	})
}

// TestQueue_RequeueIsIdempotentWhileQueued covers the window between a requeue
// and a worker picking it up.
//
// The job is in the channel and nothing is holding it yet, which is a second
// attempt just as much as a running one.
func TestQueue_RequeueIsIdempotentWhileQueued(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		blocking := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, job *Job) error {
			if job.ID == testBlockerID {
				close(blocking)
			}

			<-release

			return nil
		})
		q.Start()

		t.Cleanup(q.Stop)

		// Hold the worker so the job under test stays in the channel.
		require.NoError(t, q.Submit(&Job{
			ID: testBlockerID, Type: JobTypeClip, Status: JobStatusPending,
		}))
		<-blocking

		job := &Job{ID: "waiting", Type: JobTypeClip, Status: JobStatusPending}
		require.NoError(t, q.Requeue(job))

		err := q.Requeue(job)
		require.ErrorIs(t, err, ErrJobActive,
			"a job already in the channel would be handed to a second worker")

		close(release)
		synctest.Wait()
	})
}

// TestQueue_RequeueResetsTheJob covers the fields a requeue clears.
//
// It runs without a worker so the job is not being written underneath the
// assertions. A test that read them while a worker was live would race with it,
// which is the hazard the queue's state encapsulation work is for.
func TestQueue_RequeueResetsTheJob(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)

	job := &Job{
		ID:       "reset",
		Type:     JobTypeClip,
		Status:   JobStatusCancelled,
		Progress: 40,
		Error:    "canceled",
	}
	q.Restore(job)

	require.NoError(t, q.Requeue(job))

	assert.Equal(t, JobStatusPending, job.Status, "queued for another attempt")
	assert.Equal(t, 0, job.Progress, "and progress starts over, not from where it stopped")
	assert.Empty(t, job.Error, "with no leftover from the canceled attempt")
	assert.NotEmpty(t, q.heldReason(job.ID), "and the id is queued again")
}

// TestQueue_WaitingIsClearedWithTheWorkerRegistration covers the gap between a
// worker taking an entry out of the channel and registering itself on it.
//
// Clearing the waiting mark when the entry was received left a window where
// nothing recorded the id at all: the channel no longer held it and the cancel
// entry was not set yet. A submit landing there was let through, and the job was
// then handed to a second worker.
func TestQueue_WaitingIsClearedWithTheWorkerRegistration(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})

		q := NewQueue(1, func(_ context.Context, _ *Job) error {
			close(entered)

			<-release

			return nil
		})
		q.Start()

		t.Cleanup(q.Stop)

		job := &Job{ID: testHeldID, Type: JobTypeClip, Status: JobStatusPending}
		require.NoError(t, q.Submit(job))

		// Wait for the worker to be inside the handler. That is only reachable
		// once it has both registered itself and stopped waiting, which is
		// exactly the state under test. Asserting after the worker finished would
		// pass even if the mark were only cleared at settle time.
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

// TestGetJobHandsBackACopy is the accessor contract this item exists for.
//
// The accessors used to return the queue's own pointer, so a caller could read
// or write a job's status without the lock while a worker was settling it. The
// race detector found exactly that in a test added for another item.
func TestGetJobHandsBackACopy(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Restore(&Job{ID: testCopyID, Type: JobTypeClip, Status: JobStatusPending})

	read := q.GetJob(testCopyID)
	require.NotNil(t, read)

	read.Status = JobStatusFailed
	read.Progress = 99

	after := q.GetJob(testCopyID)
	assert.Equal(t, JobStatusPending, after.Status,
		"writing to what GetJob returned must not reach the queue's entry")
	assert.Equal(t, 0, after.Progress)
}

// TestGetAllJobsHandsBackCopies covers the listing accessor, which the clip list
// and the browse pages both read while renders are running.
func TestGetAllJobsHandsBackCopies(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Restore(&Job{ID: testCopyID, Type: JobTypeClip, Status: JobStatusPending})

	jobs := q.GetAllJobs()
	require.Len(t, jobs, 1)

	jobs[0].Status = JobStatusFailed

	assert.Equal(t, JobStatusPending, q.GetJob(testCopyID).Status,
		"the listing handed out a copy, not the entry")
}

// TestSetProgressWritesUnderTheLock is the other half. The progress callback
// runs on the render's own goroutine, so a plain field write races the worker
// settling the same job.
func TestSetProgressWritesUnderTheLock(t *testing.T) {
	t.Parallel()

	q := NewQueue(1, nil)
	q.Restore(&Job{ID: testCopyID, Type: JobTypeClip, Status: JobStatusProcessing})

	updated := q.SetProgress(testCopyID, 40)
	require.NotNil(t, updated)
	assert.Equal(t, 40, updated.Progress, "the copy carries what was set")
	assert.Equal(t, 40, q.GetJob(testCopyID).Progress, "and so does the entry")

	assert.Nil(t, q.SetProgress("never-queued", 10),
		"a job the queue does not have has nowhere to record progress")
}
