// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package queue_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
)

// errRenderFailed is what the handler reports for a render that cannot finish.
var errRenderFailed = errors.New("render failed")

// startedQueue returns a started queue whose jobs are released by cleanup.
//
// Parameters:
//   - t: The test that needs the queue.
//   - workers: How many jobs may run at once.
//   - handler: Invoked for each job.
//
// Returns:
//   - work: The started queue, stopped when the test finishes.
func startedQueue(t *testing.T, workers int, handler queue.JobHandler) *queue.Queue {
	t.Helper()

	work := queue.NewQueue(workers, handler)
	work.Start(t.Context())

	t.Cleanup(work.Stop)

	return work
}

// blockedHandler returns a handler that parks every job until it is released.
//
// Parameters:
//   - t: The test that owns the handler.
//
// Returns:
//   - handler: The blocking job handler.
//   - release: Releases every parked job. It is safe to call more than once.
//   - started: Closed once a job has reached the handler at least once.
func blockedHandler(t *testing.T) (queue.JobHandler, func(), <-chan struct{}) {
	t.Helper()

	release := make(chan struct{})
	started := make(chan struct{})

	var releaseOnce sync.Once

	var startedOnce sync.Once

	unblock := func() {
		releaseOnce.Do(func() { close(release) })
	}

	t.Cleanup(unblock)

	handler := func(ctx context.Context, _ *clip.Job) error {
		startedOnce.Do(func() { close(started) })

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}

	return handler, unblock, started
}

// queueJob returns a pending clip ready to submit.
//
// Parameters:
//   - id: Clip identifier.
//
// Returns:
//   - job: A pending clip.
func queueJob(id string) *clip.Job {
	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

	return &clip.Job{
		ID:         id,
		Type:       clip.TypeClip,
		Name:       id,
		MediaID:    "100",
		MediaTitle: "Integration Movie",
		MediaType:  clip.DefaultMediaType,

		Quality:   "medium",
		StartTime: 2 * time.Second,
		Duration:  6 * time.Second,

		CreatedAt: now,
		UpdatedAt: now, InputPath: "/media/integration.mkv",

		Status: clip.StatusPending,
	}
}

// awaitStatus waits for the queue to report the submitted job in the wanted state.
//
// Parameters:
//   - t: The test that is waiting.
//   - work: Queue holding the job.
//   - want: Status to wait for.
//
// Returns:
//   - job: The job as the queue holds it once the status arrived.
func awaitStatus(t *testing.T, work *queue.Queue, want clip.Status) *clip.Job {
	t.Helper()

	require.Eventually(t, func() bool {
		job := work.GetJob("clip-1")

		return job != nil && job.Status == want
	}, 5*time.Second, time.Millisecond, "the job reached "+string(want))

	return work.GetJob("clip-1")
}

// statusTrail records the transitions a queue reports, in order.
//
// Parameters:
//   - t: The test that owns the recorder.
//
// Returns:
//   - record: The status function to register with the queue.
//   - seen: The transitions reported so far, in order.
func statusTrail(t *testing.T) (queue.StatusFunc, func() []clip.Status) {
	t.Helper()

	var mu sync.Mutex

	seen := make([]clip.Status, 0)

	record := func(job *clip.Job) {
		mu.Lock()
		defer mu.Unlock()

		seen = append(seen, job.Status)
	}

	snapshot := func() []clip.Status {
		mu.Lock()
		defer mu.Unlock()

		return append([]clip.Status(nil), seen...)
	}

	return record, snapshot
}

func TestIntegration_SubmittedJobReportsEveryTransition(t *testing.T) {
	t.Parallel()

	handler, release, started := blockedHandler(t)
	record, snapshot := statusTrail(t)

	work := startedQueue(t, 1, handler)
	work.SetStatusFunc(record)

	require.NoError(t, work.Submit(queueJob("clip-1")))

	<-started

	require.Eventually(t, func() bool {
		job := work.GetJob("clip-1")

		return job != nil && job.Status == clip.StatusProcessing
	}, 5*time.Second, time.Millisecond, "a worker picked the job up")

	release()

	settled := awaitStatus(t, work, clip.StatusCompleted)
	assert.Equal(t, 100, settled.Progress)
	assert.Empty(t, settled.Error)

	assert.Equal(
		t,
		[]clip.Status{clip.StatusPending, clip.StatusProcessing, clip.StatusCompleted},
		snapshot(),
	)
}

func TestIntegration_SetProgressUpdatesTheLiveJob(t *testing.T) {
	t.Parallel()

	handler, release, started := blockedHandler(t)
	work := startedQueue(t, 1, handler)

	require.NoError(t, work.Submit(queueJob("clip-1")))
	<-started

	assert.Nil(t, work.SetProgress("absent", 10), "an id the queue never took has no progress")

	updated := work.SetProgress("clip-1", 42)
	require.NotNil(t, updated)
	assert.Equal(t, 42, updated.Progress)
	assert.Equal(t, 42, work.GetJob("clip-1").Progress)

	release()

	settled := awaitStatus(t, work, clip.StatusCompleted)
	assert.Equal(t, 100, settled.Progress, "completion overrides whatever the render reported")
}

func TestIntegration_CancelStopsARunningJobAndIsRefusedAfterwards(t *testing.T) {
	t.Parallel()

	handler, _, started := blockedHandler(t)
	work := startedQueue(t, 1, handler)

	require.NoError(t, work.Submit(queueJob("clip-1")))
	<-started

	assert.True(t, work.Cancel("clip-1"))

	canceled := awaitStatus(t, work, clip.StatusCancelled)
	assert.Equal(t, "canceled by the user", canceled.Error,
		"a canceled clip explains itself rather than repeating its status")
	assert.False(t, work.Cancel("clip-1"), "a settled job is not cancellable")
	assert.False(t, work.Cancel("absent"))
}

func TestIntegration_CancelRemovesAPendingJobBeforeItStarts(t *testing.T) {
	t.Parallel()

	// No worker takes from the line, so the job is still waiting when it is canceled.
	handler, _, _ := blockedHandler(t)
	work := startedQueue(t, 0, handler)

	require.NoError(t, work.Submit(queueJob("clip-1")))
	assert.True(t, work.Cancel("clip-1"))

	job := work.GetJob("clip-1")
	require.NotNil(t, job)
	assert.Equal(t, clip.StatusCancelled, job.Status)
}

func TestIntegration_DoubleSubmitIsRefusedWhileTheJobIsHeld(t *testing.T) {
	t.Parallel()

	handler, release, started := blockedHandler(t)
	work := startedQueue(t, 1, handler)

	require.NoError(t, work.Submit(queueJob("clip-1")))
	<-started

	err := work.Submit(queueJob("clip-1"))
	require.ErrorIs(t, err, queue.ErrJobActive)
	require.ErrorContains(t, err, "rendering")

	release()

	awaitStatus(t, work, clip.StatusCompleted)

	require.NoError(t, work.Submit(queueJob("clip-1")),
		"a settled job no longer holds its id")
}

func TestIntegration_QueueInFlightIsRefusedAsAlreadyQueued(t *testing.T) {
	t.Parallel()

	handler, _, _ := blockedHandler(t)
	work := startedQueue(t, 0, handler)

	require.NoError(t, work.Submit(queueJob("clip-1")))

	err := work.Submit(queueJob("clip-1"))
	require.ErrorIs(t, err, queue.ErrJobActive)
	require.ErrorContains(t, err, "already queued")

	err = work.Requeue(queueJob("clip-1"))
	require.ErrorIs(t, err, queue.ErrJobActive)
}

func TestIntegration_RestoreRegistersSettledJobsWithoutRunningThem(t *testing.T) {
	t.Parallel()

	var handled atomic.Int64

	handler := func(context.Context, *clip.Job) error {
		handled.Add(1)

		return nil
	}

	work := startedQueue(t, 1, handler)

	pending := queueJob("clip-pending")

	completed := queueJob("clip-completed")

	completed.Status = clip.StatusCompleted
	completed.Progress = 100
	completed.CreatedAt = pending.CreatedAt.Add(-time.Minute)

	older := queueJob("clip-older")

	older.Status = clip.StatusCompleted
	older.Progress = 100
	older.CreatedAt = completed.CreatedAt.Add(-time.Minute)

	work.Restore(completed)
	work.Restore(pending)
	work.Restore(older)

	all := work.GetAllJobs()
	require.Len(t, all, 3)
	assert.Equal(
		t,
		[]string{"clip-pending", "clip-completed", "clip-older"},
		clipIDs(all),
		"restored jobs are listed newest first without being enqueued",
	)

	assert.Zero(t, handled.Load(), "Restore never runs a handler")

	assert.False(t, work.Cancel("clip-completed"), "a restored settled job is not cancellable")
	assert.True(t, work.Cancel("clip-pending"))

	canceled := work.GetJob("clip-pending")
	require.NotNil(t, canceled)
	assert.Equal(t, clip.StatusCancelled, canceled.Status,
		"a restored pending job cancels without a worker ever seeing it")
}

func TestIntegration_RequeueRerunsASettledJob(t *testing.T) {
	t.Parallel()

	var attempts int

	var attemptsMu sync.Mutex

	handler := func(context.Context, *clip.Job) error {
		attemptsMu.Lock()
		defer attemptsMu.Unlock()

		attempts++

		if attempts == 1 {
			return errRenderFailed
		}

		return nil
	}

	work := startedQueue(t, 1, handler)

	job := queueJob("clip-1")
	require.NoError(t, work.Submit(job))

	failed := awaitStatus(t, work, clip.StatusFailed)
	assert.Contains(t, failed.Error, errRenderFailed.Error())

	require.NoError(t, work.Requeue(job))
	assert.Equal(t, clip.StatusPending, work.GetJob("clip-1").Status,
		"a requeued job starts over from no progress")
	assert.Empty(t, work.GetJob("clip-1").Error)

	settled := awaitStatus(t, work, clip.StatusCompleted)
	assert.Equal(t, 100, settled.Progress)
	assert.Empty(t, settled.Error, "a successful retry clears the recorded failure")
}

func TestIntegration_DeleteLeavesNoTraceBehindTheWorker(t *testing.T) {
	t.Parallel()

	handler, release, started := blockedHandler(t)
	record, snapshot := statusTrail(t)

	work := startedQueue(t, 1, handler)
	work.SetStatusFunc(record)

	require.NoError(t, work.Submit(queueJob("clip-1")))
	<-started

	work.Delete("clip-1")
	assert.Nil(t, work.GetJob("clip-1"))

	reported := len(snapshot())

	assert.Nil(t, work.SetProgress("clip-1", 50), "a deleted job has nowhere to record progress")
	assert.Len(t, snapshot(), reported, "and nothing is reported for it")

	release()

	before := len(snapshot())

	require.Never(t, func() bool {
		return len(snapshot()) > before
	}, 50*time.Millisecond, time.Millisecond,
		"nothing settles a job the queue no longer holds")
}

func TestIntegration_StopClosesTheDoneChannelAndRefusesWork(t *testing.T) {
	t.Parallel()

	handler, _, _ := blockedHandler(t)
	work := queue.NewQueue(1, handler)
	work.Start(t.Context())

	work.Stop()

	select {
	case <-work.Done():
	default:
		require.Fail(t, "Stop did not close the done channel")
	}

	err := work.Submit(queueJob("clip-1"))
	require.ErrorIs(t, err, queue.ErrQueueStopped)
	require.ErrorContains(t, err, "clip-1")

	work.Stop()
}

// clipIDs returns the identifiers of clips in the order they were given.
//
// Parameters:
//   - jobs: Clips to read.
//
// Returns:
//   - ids: The identifiers, in order.
func clipIDs(jobs []*clip.Job) []string {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}

	return ids
}
