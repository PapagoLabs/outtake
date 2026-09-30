// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package queue provides a simple job queue for processing media jobs.
package queue

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/logging"
)

// JobHandler is a function that handles jobs.
type JobHandler func(ctx context.Context, job *Job) error

// Queue represents a job queue.
type Queue struct {
	workers int
	jobChan chan *Job
	jobs    map[string]*Job
	cancels map[string]context.CancelFunc
	dropped map[string]struct{}
	// deleted records a job the queue has given up on, and whether it was still
	// queued. True means no worker holds it, so it has to be stopped before it
	// starts. False means a worker is unwinding and must be left to record its
	// own outcome.
	deleted  map[string]bool
	mu       sync.RWMutex
	wg       sync.WaitGroup
	handler  JobHandler
	done     chan struct{}
	cancel   context.CancelFunc
	statusFn StatusFunc
}

// StatusFunc is called whenever a job status changes.
//
// It is called with the queue's read lock held, so it must not call back into
// the queue. Holding the lock is what makes the deleted check and the callback
// one step: releasing the lock between them would let Delete land in the gap and
// the callback would persist the job Delete had just removed.
type StatusFunc func(job *Job)

// outcome is how a job's processing ended.
type outcome int

const (
	// Recorded means the job's final status was written.
	outcomeRecorded outcome = iota
	// Canceled means the job was canceled while running.
	outcomeCanceled
	// Deleted means the job was deleted while running, so nothing is written back.
	outcomeDeleted
)

const (
	// JobChannelSize is the size of the job channel buffer.
	jobChannelSize = 100

	// ProgressDone represents 100% progress.
	progressDone = 100
)

// ErrJobPanicked is recorded against a job whose handler panicked.
//
// It is a fixed error rather than the panic value, because a job's stored error
// is shown to whoever is watching the queue and a recovered panic value is not
// something to put in front of them.
var ErrJobPanicked = errors.New("job handler panicked")

// NewQueue creates a new job queue.
func NewQueue(workers int, handler JobHandler) *Queue {
	_, cancel := context.WithCancel(context.Background())
	queue := &Queue{
		workers:  workers,
		jobChan:  make(chan *Job, jobChannelSize),
		jobs:     make(map[string]*Job),
		cancels:  make(map[string]context.CancelFunc),
		dropped:  make(map[string]struct{}),
		deleted:  make(map[string]bool),
		mu:       sync.RWMutex{},
		wg:       sync.WaitGroup{},
		handler:  handler,
		done:     make(chan struct{}),
		cancel:   cancel,
		statusFn: nil,
	}

	return queue
}

// Cancel stops a pending or processing job.
func (q *Queue) Cancel(id string) bool {
	q.mu.Lock()

	job, ok := q.jobs[id]
	if !ok || (job.Status != JobStatusPending && job.Status != JobStatusProcessing) {
		q.mu.Unlock()

		return false
	}

	if cancel, exists := q.cancels[id]; exists {
		cancel()
	}

	q.dropped[id] = struct{}{}
	job.Status = JobStatusCancelled
	job.Error = string(JobStatusCancelled)
	job.UpdatedAt = time.Now()
	q.mu.Unlock()

	q.notify(job)

	return true
}

// Delete removes a job from the in-memory map.
//
// It records a tombstone rather than simply dropping the job. A worker that is
// already running still holds the pointer and will write the job's result on the
// way out, so without a marker it cannot tell the row was deleted and that write
// puts the row back. The tombstone outlives the delete and is cleared by the
// worker when it finishes.
func (q *Queue) Delete(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, exists := q.jobs[id]
	if !exists {
		return
	}

	// Captured before the cleanup below takes them away, because either one means
	// a worker may still be reaching for this job.
	_, running := q.cancels[id]
	_, wasDropped := q.dropped[id]

	if cancel, ok := q.cancels[id]; ok {
		cancel()
		delete(q.cancels, id)
	}

	// reached: Pending and processing jobs, and any carrying a live marker —
	// canceled while still waiting in the channel, or mid-unwind in a worker.
	//
	// A job that has already settled, or that Restore put in the map without
	// enqueueing it, has no worker coming at all. Marking those would leave the
	// entry behind for the life of the process, since the delete that would
	// clear it returns early once the job has left the map.
	reachable := job.Status == JobStatusPending || job.Status == JobStatusProcessing ||
		running || wasDropped
	if reachable {
		// A live cancel entry means a worker is holding the job, so whatever
		// settles it is that worker. Without one the job is still in the channel
		// and has to be stopped before it starts, which is what the flag records.
		q.deleted[id] = !running
	}

	// The canceled marker goes too, now that the tombstone is what stops a job
	// still waiting in the channel from running. Clearing it here is what stops
	// the marker outliving the worker that would have removed it.
	delete(q.dropped, id)
	delete(q.jobs, id)
}

// Done returns a channel that is closed when the queue is stopped.
func (q *Queue) Done() <-chan struct{} {
	return q.done
}

// GetAllJobs returns every job, newest first.
//
// Jobs with the same CreatedAt are ordered by ID descending.
func (q *Queue) GetAllJobs() []*Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := slices.Collect(maps.Values(q.jobs))
	slices.SortFunc(result, compareJobsNewestFirst)

	return result
}

// compareJobsNewestFirst orders jobs by CreatedAt descending, then ID descending.
func compareJobsNewestFirst(left, right *Job) int {
	if order := right.CreatedAt.Compare(left.CreatedAt); order != 0 {
		return order
	}

	return cmp.Compare(right.ID, left.ID)
}

// GetJob gets a job by ID.
func (q *Queue) GetJob(id string) *Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.jobs[id]
}

// IfLive runs fn only while the queue still owns the job, holding the read
// lock for its duration.
//
// It is the same one-step guarantee notify gets, for a caller that persists the
// job rather than observing it. A delete cannot land between the check and fn, so
// a job removed in that window is not written back.
//
// Like the status callback, fn must not call back into the queue.
//
// Parameters:
//   - id: Job the caller is about to write.
//   - fn: The write.
func (q *Queue) IfLive(id string, fn func()) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if _, deleted := q.deleted[id]; deleted {
		return
	}

	fn()
}

// Reinstate puts a job back after a delete that could not be completed.
//
// Delete takes the job out of the queue and marks it before the row is removed,
// so that a status change cannot write the row back in between. If the row
// removal then fails, the job has to come back: left out, the row has nothing
// owning it, a cancel would not find it, and the next start would resubmit a
// render the delete had already stopped.
//
// It comes back canceled rather than as it was. The render is not resuming, and
// a job that never started must not be picked up again on the next boot. A
// worker that is still unwinding will settle it afterwards and record whatever
// the render actually ended as, which is the honest answer.
//
// Parameters:
//   - job: The job Delete took.
func (q *Queue) Reinstate(job *Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// The marker is cleared, but a job still sitting in the channel has nothing
	// else stopping it from starting now, so it becomes a canceled one instead.
	// A job a worker is already unwinding must not be: marking it would make
	// that worker discard a real outcome, and setting its status here would be
	// overwritten anyway.
	queued := q.deleted[job.ID]
	delete(q.deleted, job.ID)
	delete(q.dropped, job.ID)

	if queued {
		q.dropped[job.ID] = struct{}{}
	}

	job.Status = JobStatusCancelled
	job.Error = string(JobStatusCancelled)
	job.UpdatedAt = time.Now()
	q.jobs[job.ID] = job
}

// Restore registers a job without enqueueing it.
func (q *Queue) Restore(job *Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.jobs[job.ID] = job
}

// SetStatusFunc registers a callback invoked on job status changes.
func (q *Queue) SetStatusFunc(fn StatusFunc) {
	q.statusFn = fn
}

// Start starts the job queue workers.
func (q *Queue) Start() {
	for i := range q.workers {
		q.wg.Go(func() {
			q.worker(i)
		})
	}

	logging.Logger.Info().Int("workers", q.workers).Msg("job queue started")
}

// Stop stops the job queue.
func (q *Queue) Stop() {
	q.cancel()
	close(q.jobChan)
	close(q.done)
	q.wg.Wait()
}

// Submit submits a job to the queue.
func (q *Queue) Submit(job *Job) {
	q.mu.Lock()

	q.jobs[job.ID] = job
	q.mu.Unlock()

	q.jobChan <- job

	q.notify(job)

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("job submitted")
}

// notify invokes the status callback when one is registered.
//
// A deleted job is skipped. The callback persists the job, and the queue no
// longer owns one it deleted, so writing it back would put the row Delete
// removed. The tombstone is left for the running worker to clear.
//
// The check and the callback are one step under a single read lock. Releasing
// the lock between them would leave a gap that Delete could land in, and the
// callback would then persist the job after it had been removed — which is the
// same resurrection the tombstone exists to prevent, arriving by a different
// route. See StatusFunc for what holding the lock asks of the callback.
func (q *Queue) notify(job *Job) {
	if q.statusFn == nil {
		return
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	if _, deleted := q.deleted[job.ID]; deleted {
		return
	}

	q.statusFn(job)
}

// processJob processes a single job.
func (q *Queue) processJob(job *Job) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q.mu.Lock()

	if _, gone := q.deleted[job.ID]; gone {
		// Deleted before it ever started. Both markers go, so neither outlives
		// the worker that would have removed it.
		delete(q.deleted, job.ID)
		delete(q.dropped, job.ID)
		q.mu.Unlock()

		return
	}

	if _, dropped := q.dropped[job.ID]; dropped {
		delete(q.dropped, job.ID)
		q.mu.Unlock()

		return
	}

	job.Status = JobStatusProcessing
	job.UpdatedAt = time.Now()
	q.cancels[job.ID] = cancel
	q.mu.Unlock()

	q.notify(job)

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("processing job")

	err := q.runHandler(ctx, job)

	result := q.settle(job, err)

	switch {
	case result == outcomeDeleted:
		logging.Logger.Info().Str("job_id", job.ID).Msg("job deleted while processing")
	case result == outcomeCanceled:
		logging.Logger.Info().Str("job_id", job.ID).Msg("job canceled")
	case err != nil:
		logging.Logger.Error().
			Str("job_id", job.ID).
			Err(err).
			Msg("job failed")
	default:
		logging.Logger.Info().
			Str("job_id", job.ID).
			Msg("job completed")
	}

	if result == outcomeDeleted {
		return
	}

	q.notify(job)
}

// runHandler invokes the job handler, turning a panic into an error.
//
// A panic in a worker goroutine takes the process down with it, and the handler
// is the only part of a worker that runs code this package does not control:
// ffmpeg, the database and object storage all sit behind it. Recovering keeps one
// bad job from ending the run, and the stack goes to the log where it is useful
// rather than into the job's error where it is not.
//
// The job is recorded as failed rather than retried. A panic is a programming
// error rather than a transient condition, so retrying would put the same panic
// on a timer.
//
// Parameters:
//   - ctx: Cancellation for the job.
//   - job: The job being handled.
//
// Returns:
//   - err: The handler's error, or ErrJobPanicked if it panicked.
//
// A deferred recover can only replace the result through a named return. It is
// the one place the pattern is the reason rather than a convenience.
//
//nolint:nonamedreturns // a deferred recover writes the result through the name.
func (q *Queue) runHandler(ctx context.Context, job *Job) (err error) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		// The stack is captured here rather than through zerolog's Stack, which
		// only renders when an error is attached and this event carries none. A
		// panic with no stack is the one thing that makes this log untrustworthy,
		// so the trace is taken explicitly while the frames are still live.
		logging.Logger.Error().
			Str("job_id", job.ID).
			Str("type", string(job.Type)).
			Interface("panic", recovered).
			Str("stack", string(debug.Stack())).
			Msg("job handler panicked")

		err = ErrJobPanicked
	}()

	// The handler is the queue's own contract with its caller. Wrapping its error
	// here would rewrite what the job records and what the caller sees, so it is
	// passed through as it is.
	//nolint:wrapcheck // the handler's error belongs to the caller that supplied it.
	return q.handler(ctx, job)
}

// settle records how a job ended and releases its bookkeeping.
//
// The tombstones are cleared here rather than in Delete, so they live exactly as
// long as a worker might still write the job back.
//
// Parameters:
//   - job: The job that was processed.
//   - err: What the handler returned.
//
// Returns:
//   - outcome: How the job ended.
func (q *Queue) settle(job *Job, err error) outcome {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, canceled := q.dropped[job.ID]
	_, gone := q.deleted[job.ID]
	delete(q.dropped, job.ID)
	delete(q.deleted, job.ID)
	delete(q.cancels, job.ID)

	if gone {
		// The row was deleted while this was running, so the result is not
		// written back. The status is left as the handler left it rather than
		// being recorded, since there is no longer a job to record it against.
		return outcomeDeleted
	}

	switch {
	case canceled:
		job.Status = JobStatusCancelled
		job.Error = string(JobStatusCancelled)
	case err != nil:
		job.Status = JobStatusFailed
		job.Error = err.Error()
	default:
		job.Status = JobStatusCompleted
		// Cleared, because a job can arrive here still carrying an error. A
		// reinstated one is marked canceled while a worker unwinds, and that
		// worker may then report success — leaving a completed job whose error
		// says it was canceled.
		job.Error = ""
		job.Progress = progressDone
	}

	job.UpdatedAt = time.Now()

	if canceled {
		return outcomeCanceled
	}

	return outcomeRecorded
}

// worker processes jobs from the queue.
func (q *Queue) worker(_ int) {
	for {
		select {
		case <-q.done:
			return
		case job, ok := <-q.jobChan:
			if !ok {
				return
			}

			q.processJob(job)
		}
	}
}
