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
	// waiting records the ids with an entry sitting in the channel, not yet
	// picked up. Reachability is tracked here rather than read off a status,
	// because a caller legitimately writes Pending while re-queueing an idle job
	// and that must not look like a second job for the same id.
	waiting map[string]struct{}
	// deleted records a job the queue has given up on, and whether it was still
	// queued. True means no worker holds it, so it has to be stopped before it
	// starts. False means a worker is unwinding and must be left to record its
	// own outcome.
	deleted map[string]bool
	// stop guards the one-time teardown, so a second Stop or a Stop racing a
	// Submit is a no-op rather than a panic on a closed channel.
	stop sync.Once
	// stopped is set under the lock when the queue is torn down, so a submit can
	// refuse deterministically rather than relying on a select against the done
	// channel. A stopped queue still has buffer room, so a send would succeed
	// into a buffer nothing is left to drain.
	stopped  bool
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
	// ErrQueueStoppedFormat wraps ErrQueueStopped with the id it refused.
	errQueueStoppedFormat = "%w: %s"
	// JobChannelSize is the size of the job channel buffer.
	jobChannelSize = 100

	// ProgressDone represents 100% progress.
	progressDone = 100
	// ProgressReset is the progress a re-queued job starts from.
	progressReset = 0
)

// ErrJobActive is returned when a job is submitted while one with the same id
// is already queued or running.
var ErrJobActive = errors.New("a job with this id is already active")

// ErrQueueStopped is returned when a job is submitted to a stopped queue.
var ErrQueueStopped = errors.New("queue is stopped")

// ErrJobPanicked is recorded against a job whose handler panicked.
//
// It is a fixed error rather than the panic value, because a job's stored error
// is shown to whoever is watching the queue and a recovered panic value is not
// something to put in front of them.
var ErrJobPanicked = errors.New("job handler panicked")

// NewQueue creates a new job queue.
//
// The queue's lifetime follows ctx. Canceling it stops the queue the same way
// Stop does, so a caller that already owns a shutdown signal does not need to
// know about the queue at all.
//
// Parameters:
//   - ctx: Lifetime for the queue and for every job it runs.
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
		jobChan:  make(chan *Job, jobChannelSize),
		jobs:     make(map[string]*Job),
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
// Jobs with the same CreatedAt are ordered by ID descending. Each is a copy, so
// the caller can read it without the lock and without racing a worker that is
// settling the job underneath.
func (q *Queue) GetAllJobs() []*Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]*Job, 0, len(q.jobs))
	for _, job := range q.jobs {
		result = append(result, job.clone())
	}

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

// GetJob gets a job by ID, as a copy.
//
// The copy is what makes it safe to read the returned job's fields without the
// queue's lock: a worker mutates status and progress in place, so handing out
// the queue's own pointer would let a reader tear. A caller that wants to change
// a job persists its copy and, where it is still live, hands it back through
// Submit or Requeue rather than writing through the pointer.
func (q *Queue) GetJob(id string) *Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.jobs[id].clone()
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

// Requeue puts an idle job back on the queue for another attempt.
//
// The reset happens under the same lock as the check, because the caller cannot
// safely do it first: it writes Pending, which is the very state the check reads,
// so it would report the job it just reset as a second job for its own id. Doing
// it after leaves a refusal having changed a job the queue then refused to take,
// which the map would keep showing as pending with nothing running it.
//
// Parameters:
//   - job: Job to run again.
//
// Returns:
//   - err: ErrJobActive when a worker holds the job or the channel already has it.
func (q *Queue) Requeue(job *Job) error {
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
	// job and queueing it. A value copy is complete here for the same reason it
	// is on the accessors.
	previous := *job

	job.Status = JobStatusPending
	job.Progress = progressReset
	job.Error = ""
	job.UpdatedAt = time.Now()

	// A cancellation marker left by a job that was never picked up would
	// otherwise skip this one instead. Re-running is the explicit intent, and
	// there is no worker here to have cleared it: one that had picked the job up
	// would be holding it, which is refused above.
	delete(q.dropped, job.ID)

	q.jobs[job.ID] = job
	q.waiting[job.ID] = struct{}{}
	q.mu.Unlock()

	// Reported before the entry is on the channel. The worker notifies the moment
	// it picks the job up, so enqueuing first could let it record processing and
	// have this write pending over the top — the newer state losing to the older.
	q.notify(job)

	select {
	case q.jobChan <- job:
	case <-q.done:
		// Shutdown between taking the job and queueing it. The job goes back to
		// what it said before this call, in the queue and in the row. Leaving it
		// reset would be the stranded case again: pending, with nothing running
		// it and nothing queued to pick it up.
		restored := previous.clone()

		q.mu.Lock()
		delete(q.waiting, job.ID)

		q.jobs[job.ID] = restored
		q.mu.Unlock()

		q.notify(restored)

		return fmt.Errorf(errQueueStoppedFormat, ErrQueueStopped, job.ID)
	}

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("job requeued")

	return nil
}

// Restore registers a job without enqueueing it.
func (q *Queue) Restore(job *Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.jobs[job.ID] = job
}

// SetProgress records how far a render has got.
//
// The write happens under the lock, because the progress callback runs on the
// render's own goroutine while a worker settles the same job from another. The
// updated job is returned as a copy so the caller can persist it without touching
// the queue's entry.
//
// Parameters:
//   - id: Job being rendered.
//   - percent: Progress so far.
//
// Returns:
//   - job: The updated job, nil when the queue no longer has it.
func (q *Queue) SetProgress(id string, percent int) *Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok {
		return nil
	}

	job.Progress = percent
	job.UpdatedAt = time.Now()

	return job.clone()
}

// SetStatusFunc registers a callback invoked on job status changes.
func (q *Queue) SetStatusFunc(fn StatusFunc) {
	q.statusFn = fn
}

// Start starts the job queue workers.
//
// The context is the lifetime of the queue and of every job it runs, so a
// caller that already holds a shutdown signal does not need to know about the
// queue to stop it. Canceling it is equivalent to Stop, and it is what lets a
// running ffmpeg be torn down rather than abandoned.
//
// Parameters:
//   - ctx: Lifetime for the queue and for every job.
func (q *Queue) Start(ctx context.Context) {
	// Stop cancels this rather than the context it was built from, because this is
	// the one every job's context descends from. Canceling the caller's context
	// instead would be wrong — Stop does not own it — and would leave a running
	// ffmpeg unwinding on nothing.
	ctx, cancel := context.WithCancel(ctx)

	q.mu.Lock()

	q.cancel = cancel
	q.mu.Unlock()

	// Canceling the caller's context stops the queue through the same guarded
	// path Stop uses, rather than only canceling the jobs in flight. Without it
	// the workers would sit waiting on the channel with nothing left to feed
	// them, and a later Submit would be accepted by a queue that can never run
	// it.
	context.AfterFunc(ctx, q.Stop)

	for i := range q.workers {
		q.wg.Go(func() {
			q.worker(ctx, i)
		})
	}

	logging.Logger.Info().Int("workers", q.workers).Msg("job queue started")
}

// Stop stops the job queue and waits for its workers.
//
// It is safe to call more than once and to race a Submit. Canceling the
// queue's context is what stops a running job, so an in-flight ffmpeg is torn
// down rather than abandoned.
//
// The job channel is deliberately left open. Closing it would be a second exit
// for the workers, and it is the one Submit sends on, so closing it turns a
// concurrent Submit into a panic. The workers leave on the done channel instead,
// and Submit refuses rather than sending.
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
// It refuses a second job for an id that is already queued or running. Two
// workers on one id would render the same output path at once and race on the
// status the first one writes, so a caller wanting to re-render has to wait for
// the current attempt rather than overlap it.
//
// Parameters:
//   - job: Job to queue.
//
// Returns:
//   - err: ErrJobActive when the id is already queued or running.
func (q *Queue) Submit(job *Job) error {
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
	// Requeue: a worker that picks the job up first would record processing, and
	// this would then write pending over it.
	q.notify(job)

	// Waiting on the shutdown as well as the channel. The stopped check above
	// catches a queue that is already down; this covers the narrow window where
	// it goes down between that check and this send, and a full buffer would
	// otherwise leave the submit waiting on a queue whose workers may all have
	// left without draining it.
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
// It must be called with the lock held, so the answer is taken before the lock
// is released rather than by reading the job afterwards.
//
// Parameters:
//   - id: Job id to check.
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
func (q *Queue) processJob(ctx context.Context, job *Job) {
	// The worker's own context, so a single job can be canceled without
	// disturbing the rest of the queue.
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	q.mu.Lock()

	// The entry is in a worker's hands now, so the queue no longer has it
	// waiting. This is cleared here rather than where the entry was received,
	// because between those two locks nothing records the id at all: the channel
	// has let it go and the cancel entry is not set until below. A submit landing
	// in that gap was let through, and the job went to a second worker.
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

	job.Status = JobStatusProcessing
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

	// The entry settle recorded against, not the object this worker captured.
	// A reinstate replaces the entry with a caller's copy, so notifying the
	// captured one would persist a status the settle had already moved on from.
	q.notify(settled)
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
//   - settled: The entry the outcome was recorded against, nil when the job was
//     deleted while running and nothing should be written.
func (q *Queue) settle(job *Job, err error) (outcome, *Job) {
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
	// belongs on that one rather than on the object this worker captured. A
	// reinstate hands the job back as a copy from the caller, and writing to the
	// captured object would record the result against something the queue has
	// already dropped.
	live, tracked := q.jobs[job.ID]
	if !tracked {
		live = job
	}

	switch {
	case canceled:
		live.Status = JobStatusCancelled
		live.Error = string(JobStatusCancelled)
	case err != nil:
		live.Status = JobStatusFailed
		live.Error = err.Error()
	default:
		live.Status = JobStatusCompleted
		// Cleared, because a job can arrive here still carrying an error. A
		// reinstated one is marked canceled while a worker unwinds, and that
		// worker may then report success — leaving a completed job whose error
		// says it was canceled.
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
