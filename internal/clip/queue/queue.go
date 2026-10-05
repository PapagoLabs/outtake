// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package queue provides a simple job queue for processing media jobs.
package queue

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/logging"
)

// JobHandler is a function that handles jobs.
type JobHandler func(ctx context.Context, job *clip.Job) error

// Queue represents a job queue.
type Queue struct {
	workers  int
	jobChan  chan *clip.Job
	jobs     map[string]*clip.Job
	cancels  map[string]context.CancelFunc
	dropped  map[string]struct{}
	waiting  map[string]struct{}
	deleted  map[string]bool
	stop     sync.Once
	stopped  bool
	mu       sync.RWMutex
	wg       sync.WaitGroup
	handler  JobHandler
	done     chan struct{}
	cancel   context.CancelFunc
	statusFn StatusFunc
}

// StatusFunc is called whenever a job status changes.
type StatusFunc func(job *clip.Job)

// outcome is how a job's processing ended.
type outcome int

const (
	// outcomeRecorded means the job's final status was written.
	outcomeRecorded outcome = iota
	// outcomeCanceled means the job was canceled while running.
	outcomeCanceled
	// outcomeDeleted means the job was deleted while running, so nothing is written back.
	outcomeDeleted
)

const (
	// errQueueStoppedFormat wraps ErrQueueStopped with the id it refused.
	errQueueStoppedFormat = "%w: %s"
	// jobChannelSize is the size of the job channel buffer.
	jobChannelSize = 100

	// progressDone is the progress value that means a job is finished.
	progressDone = 100
	// progressReset is the progress a re-queued job starts from.
	progressReset = 0

	// canceledMessage is the failure message a canceled job carries. It is a
	// sentence rather than the status literal, because clip.Error is rendered
	// verbatim to the user alongside the reason a render failed.
	canceledMessage = "canceled by the user"
)

// ErrJobActive is returned when a job is submitted while one with the same id is already queued or running.
var ErrJobActive = errors.New("a job with this id is already active")

// ErrQueueStopped is returned when a job is submitted to a stopped queue.
var ErrQueueStopped = errors.New("queue is stopped")

// ErrJobPanicked is recorded against a job whose handler panicked.
var ErrJobPanicked = errors.New("job handler panicked")

// NewQueue creates a new job queue.
//
// Parameters:
//   - workers: How many jobs may run at once.
//   - handler: Invoked for each job.
//
// Returns:
//   - queue: A queue ready to Start.
func NewQueue(workers int, handler JobHandler) *Queue {
	_, cancel := context.WithCancel(context.Background())
	queue := &Queue{
		stop:     sync.Once{},
		workers:  workers,
		jobChan:  make(chan *clip.Job, jobChannelSize),
		jobs:     make(map[string]*clip.Job),
		cancels:  make(map[string]context.CancelFunc),
		dropped:  make(map[string]struct{}),
		waiting:  make(map[string]struct{}),
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
//
// Parameters:
//   - id: The clip to cancel.
//
// Returns:
//   - canceled: True when a pending or processing job was canceled.
func (q *Queue) Cancel(id string) bool {
	q.mu.Lock()

	job, ok := q.jobs[id]
	if !ok || (job.Status != clip.StatusPending && job.Status != clip.StatusProcessing) {
		q.mu.Unlock()

		return false
	}

	if cancel, exists := q.cancels[id]; exists {
		cancel()
	}

	q.dropped[id] = struct{}{}
	job.Status = clip.StatusCancelled
	job.Error = canceledMessage
	job.UpdatedAt = time.Now()
	q.mu.Unlock()

	q.notify(job)

	return true
}

// Delete removes a job from the in-memory map.
//
// Parameters:
//   - id: The clip to remove.
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

	// reached: Pending and processing jobs, and any carrying a live marker.
	//
	// A job that has already settled, or that Restore put in the map without
	// enqueueing it, has no worker coming at all. Marking those would leave the
	// entry behind for the life of the process.
	reachable := job.Status == clip.StatusPending || job.Status == clip.StatusProcessing ||
		running || wasDropped
	if reachable {
		// A live cancel entry means a worker is holding the job, so whatever
		// settles it is that worker. Without one the job is still in the channel
		// and has to be stopped before it starts, which is what the flag records.
		q.deleted[id] = !running
	}

	// The canceled marker goes too, now that the tombstone is what stops a job
	// still waiting in the channel from running.
	delete(q.dropped, id)
	delete(q.jobs, id)
}

// Done returns a channel that is closed when the queue is stopped.
//
// Returns:
//   - done: Closed on teardown.
func (q *Queue) Done() <-chan struct{} {
	return q.done
}

// GetAllJobs returns every job, newest first.
//
// Returns:
//   - jobs: Copies of every registered job.
func (q *Queue) GetAllJobs() []*clip.Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]*clip.Job, 0, len(q.jobs))
	for _, job := range q.jobs {
		result = append(result, job.Clone())
	}

	slices.SortFunc(result, compareJobsNewestFirst)

	return result
}

// compareJobsNewestFirst orders jobs by CreatedAt descending, then ID descending.
//
// Parameters:
//   - left: The first job to order.
//   - right: The second job to order.
//
// Returns:
//   - order: Negative when left sorts first, positive when right does.
func compareJobsNewestFirst(left, right *clip.Job) int {
	if order := right.CreatedAt.Compare(left.CreatedAt); order != 0 {
		return order
	}

	return cmp.Compare(right.ID, left.ID)
}

// GetJob gets a job by ID, as a copy.
//
// Parameters:
//   - id: The clip to read.
//
// Returns:
//   - job: A copy of the job, nil when the queue does not have it.
func (q *Queue) GetJob(id string) *clip.Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.jobs[id].Clone()
}

// IfLive runs fn only while the queue still owns the job, holding the read
// lock for its duration.
//
// Parameters:
//   - id: The clip the caller is about to write.
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
// Parameters:
//   - job: The job Delete took.
func (q *Queue) Reinstate(job *clip.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// The marker is cleared, but a job still sitting in the channel has nothing
	// else stopping it from starting now, so it becomes a canceled one instead.
	queued := q.deleted[job.ID]
	delete(q.deleted, job.ID)
	delete(q.dropped, job.ID)

	if queued {
		// Only a job still waiting in the channel has to be stopped. A finished
		// clip, or one a worker is already settling, keeps the status it had.
		q.dropped[job.ID] = struct{}{}
		job.Status = clip.StatusCancelled
		job.Error = canceledMessage
	}

	job.UpdatedAt = time.Now()
	q.jobs[job.ID] = job
}

// Requeue puts an idle job back on the queue for another attempt.
//
// Parameters:
//   - job: The clip to run again.
//
// Returns:
//   - err: ErrJobActive when a worker holds the job or the channel already has it.
func (q *Queue) Requeue(job *clip.Job) error {
	q.mu.Lock()

	if q.stopped {
		q.mu.Unlock()

		return fmt.Errorf(errQueueStoppedFormat, ErrQueueStopped, job.ID)
	}

	if reason := q.heldReason(job.ID); reason != "" {
		q.mu.Unlock()

		return fmt.Errorf("%w: %s is %s", ErrJobActive, job.ID, reason)
	}

	// Kept so the reset below can be undone if the queue stops between taking the
	// job and queueing it.
	previous := *job

	job.Status = clip.StatusPending
	job.Progress = progressReset
	job.Error = ""
	job.UpdatedAt = time.Now()

	// A cancellation marker left by a job that was never picked up would
	// otherwise skip this one instead.
	delete(q.dropped, job.ID)

	q.jobs[job.ID] = job
	q.waiting[job.ID] = struct{}{}
	q.mu.Unlock()

	// Reported before the entry is on the channel, so a worker that picks the job
	// up first cannot have this write land over its processing status.
	q.notify(job)

	select {
	case q.jobChan <- job:
	case <-q.done:
		// Shutdown between taking the job and queueing it. The job goes back to
		// what it said before this call, so it is not left pending with nothing
		// running it and nothing queued to pick it up.
		q.mu.Lock()
		delete(q.waiting, job.ID)

		*job = previous
		q.jobs[job.ID] = job
		q.mu.Unlock()

		q.notify(job)

		return fmt.Errorf(errQueueStoppedFormat, ErrQueueStopped, job.ID)
	}

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("job requeued")

	return nil
}

// Restore registers a job without enqueueing it.
//
// Parameters:
//   - job: The clip to track without running.
func (q *Queue) Restore(job *clip.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.jobs[job.ID] = job
}

// SetProgress records how far a render has got.
//
// Parameters:
//   - id: The clip being rendered.
//   - percent: Progress so far.
//
// Returns:
//   - job: The updated job, nil when the queue no longer has it.
func (q *Queue) SetProgress(id string, percent int) *clip.Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok {
		return nil
	}

	job.Progress = percent
	job.UpdatedAt = time.Now()

	return job.Clone()
}

// SetStatusFunc registers a callback invoked on job status changes.
//
// Parameters:
//   - fn: The callback, or nil to stop reporting.
func (q *Queue) SetStatusFunc(fn StatusFunc) {
	q.statusFn = fn
}

// Start starts the job queue workers.
//
// Parameters:
//   - ctx: Lifetime for the queue and for every job.
func (q *Queue) Start(ctx context.Context) {
	// Stop cancels this rather than the context it was built from, because this is
	// the one every job's context descends from. Canceling the caller's context
	// instead would leave a running ffmpeg unwinding on nothing.
	ctx, cancel := context.WithCancel(ctx)

	q.mu.Lock()

	q.cancel = cancel
	q.mu.Unlock()

	// Canceling the caller's context stops the queue through the same guarded
	// path Stop uses. Without it a later Submit would be accepted by a queue
	// that can never run it.
	context.AfterFunc(ctx, q.Stop)

	for i := range q.workers {
		q.wg.Go(func() {
			q.worker(ctx, i)
		})
	}

	logging.Logger.Info().Int("workers", q.workers).Msg("job queue started")
}

// Stop stops the job queue and waits for its workers.
func (q *Queue) Stop() {
	q.stop.Do(func() {
		q.mu.Lock()

		q.stopped = true
		q.mu.Unlock()

		q.cancel()
		close(q.done)
	})

	q.wg.Wait()
}

// Submit submits a job to the queue.
//
// Parameters:
//   - job: The clip to queue.
//
// Returns:
//   - err: ErrJobActive when the id is already queued or running.
func (q *Queue) Submit(job *clip.Job) error {
	q.mu.Lock()

	if q.stopped {
		q.mu.Unlock()

		return fmt.Errorf(errQueueStoppedFormat, ErrQueueStopped, job.ID)
	}

	if reason := q.heldReason(job.ID); reason != "" {
		q.mu.Unlock()

		return fmt.Errorf("%w: %s is %s", ErrJobActive, job.ID, reason)
	}

	q.jobs[job.ID] = job
	q.waiting[job.ID] = struct{}{}
	q.mu.Unlock()

	// Reported before the entry is on the channel, for the same reason as
	// Requeue.
	q.notify(job)

	// Waiting on the shutdown as well as the channel, which covers the narrow
	// window where the queue goes down between the check above and this send.
	select {
	case q.jobChan <- job:
	case <-q.done:
		// Shutdown between taking the job and queueing it. The mark goes back,
		// since nothing is going to reach it.
		q.mu.Lock()
		delete(q.waiting, job.ID)
		q.mu.Unlock()

		return fmt.Errorf(errQueueStoppedFormat, ErrQueueStopped, job.ID)
	}

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("job submitted")

	return nil
}

// heldReason reports why an id is already taken, and whether it is.
//
// Parameters:
//   - id: The clip id to check.
//
// Returns:
//   - reason: What has the id, empty when nothing does.
func (q *Queue) heldReason(id string) string {
	if _, running := q.cancels[id]; running {
		return "rendering"
	}

	if _, waiting := q.waiting[id]; waiting {
		return "already queued"
	}

	return ""
}

// notify invokes the status callback when one is registered.
//
// Parameters:
//   - job: The job whose status changed.
func (q *Queue) notify(job *clip.Job) {
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
//
// Parameters:
//   - ctx: The worker's context, which a single job can be canceled from.
//   - job: The job to run.
func (q *Queue) processJob(ctx context.Context, job *clip.Job) {
	// The worker's own context, so a single job can be canceled without
	// disturbing the rest of the queue.
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	q.mu.Lock()

	// The entry is in a worker's hands now, so the queue no longer has it
	// waiting. This is cleared here rather than where the entry was received,
	// because between those two locks nothing records the id at all.
	delete(q.waiting, job.ID)

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

	job.Status = clip.StatusProcessing
	job.UpdatedAt = time.Now()
	q.cancels[job.ID] = cancel
	q.mu.Unlock()

	q.notify(job)

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("processing job")

	err := q.runHandler(jobCtx, job)

	result, settled := q.settle(job, err)

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

	// The entry settle recorded against, not the object this worker captured,
	// since a reinstate replaces the entry with a caller's copy.
	q.notify(settled)
}

// runHandler invokes the job handler, turning a panic into an error.
//
// Parameters:
//   - ctx: Cancellation for the job.
//   - job: The job being handled.
//
// Returns:
//   - err: The handler's error, or ErrJobPanicked if it panicked.
//
//nolint:nonamedreturns // a deferred recover writes the result through the name.
func (q *Queue) runHandler(ctx context.Context, job *clip.Job) (err error) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		// The stack is captured here rather than through zerolog's Stack, which
		// only renders when an error is attached and this event carries none.
		logging.Logger.Error().
			Str("job_id", job.ID).
			Str("type", string(job.Type)).
			Interface("panic", recovered).
			Str("stack", string(debug.Stack())).
			Msg("job handler panicked")

		err = ErrJobPanicked
	}()

	// The handler is the queue's own contract with its caller, so its error is
	// passed through as it is.
	//nolint:wrapcheck // the handler's error belongs to the caller that supplied it.
	return q.handler(ctx, job)
}

// settle records how a job ended and releases its bookkeeping.
//
// Parameters:
//   - job: The job that was processed.
//   - err: What the handler returned.
//
// Returns:
//   - outcome: How the job ended.
//   - settled: The entry the outcome was recorded against, nil when the job was
//     deleted while running and nothing should be written.
func (q *Queue) settle(job *clip.Job, err error) (outcome, *clip.Job) {
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
		return outcomeDeleted, nil
	}

	// Everything downstream reads the entry the queue holds, so the outcome
	// belongs on that one rather than on the object this worker captured.
	live, tracked := q.jobs[job.ID]
	if !tracked {
		live = job
	}

	switch {
	case q.stopped && !canceled && err != nil:
		// The queue is shutting down, so the job was interrupted rather than
		// broken. It goes back to pending with no error, which is what restoreJobs
		// looks for, so the render is picked up again on the next start.
		live.Status = clip.StatusPending
		live.Error = ""
	case canceled:
		live.Status = clip.StatusCancelled
		live.Error = canceledMessage
	case err != nil:
		live.Status = clip.StatusFailed
		live.Error = err.Error()
	default:
		live.Status = clip.StatusCompleted
		// Cleared, because a reinstated job that a worker unwinds may then
		// report success, leaving a completed job whose error says it was
		// canceled.
		live.Error = ""
		live.Progress = progressDone
	}

	live.UpdatedAt = time.Now()

	if canceled {
		return outcomeCanceled, live
	}

	return outcomeRecorded, live
}

// worker processes jobs from the queue.
//
// Parameters:
//   - ctx: The worker lifetime, canceled when the queue stops.
//   - _: Unused worker index.
func (q *Queue) worker(ctx context.Context, _ int) {
	for {
		select {
		case <-q.done:
			return
		case job, ok := <-q.jobChan:
			if !ok {
				return
			}

			q.processJob(ctx, job)
		}
	}
}
