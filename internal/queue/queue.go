// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package queue provides a simple job queue for processing media jobs.
package queue

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/logging"
)

// JobHandler is a function that handles jobs.
type JobHandler func(ctx context.Context, job *Job) error

// Queue represents a job queue.
type Queue struct {
	workers  int
	jobChan  chan *Job
	jobs     map[string]*Job
	cancels  map[string]context.CancelFunc
	dropped  map[string]struct{}
	deleted  map[string]struct{}
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

// NewQueue creates a new job queue.
func NewQueue(workers int, handler JobHandler) *Queue {
	_, cancel := context.WithCancel(context.Background())
	queue := &Queue{
		workers:  workers,
		jobChan:  make(chan *Job, jobChannelSize),
		jobs:     make(map[string]*Job),
		cancels:  make(map[string]context.CancelFunc),
		dropped:  make(map[string]struct{}),
		deleted:  make(map[string]struct{}),
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
func (que *Queue) Cancel(id string) bool {
	que.mu.Lock()

	job, ok := que.jobs[id]
	if !ok || (job.Status != JobStatusPending && job.Status != JobStatusProcessing) {
		que.mu.Unlock()

		return false
	}

	if cancel, exists := que.cancels[id]; exists {
		cancel()
	}

	que.dropped[id] = struct{}{}
	job.Status = JobStatusCancelled
	job.Error = string(JobStatusCancelled)
	job.UpdatedAt = time.Now()
	que.mu.Unlock()

	que.notify(job)

	return true
}

// Delete removes a job from the in-memory map.
//
// It records a tombstone rather than simply dropping the job. A worker that is
// already running still holds the pointer and will write the job's result on the
// way out, so without a marker it cannot tell the row was deleted and that write
// puts the row back. The tombstone outlives the delete and is cleared by the
// worker when it finishes.
func (que *Queue) Delete(id string) {
	que.mu.Lock()
	defer que.mu.Unlock()

	job, exists := que.jobs[id]
	if !exists {
		return
	}

	// Captured before the cleanup below takes them away, because either one means
	// a worker may still be reaching for this job.
	_, running := que.cancels[id]
	_, wasDropped := que.dropped[id]

	if cancel, ok := que.cancels[id]; ok {
		cancel()
		delete(que.cancels, id)
	}

	// A tombstone exists so a worker can find it and clear it, so only a job a
	// worker can still reach gets one: a pending or processing one, or one
	// carrying a live marker — canceled while still waiting in the channel, or
	// mid-unwind in a worker.
	//
	// A job that has already settled, or that Restore put in the map without
	// enqueueing it, has no worker coming at all. Tombstoning those would leave
	// the entry behind for the life of the process, since the delete that would
	// clear it returns early once the job has left the map.
	reachable := job.Status == JobStatusPending || job.Status == JobStatusProcessing ||
		running || wasDropped
	if reachable {
		que.deleted[id] = struct{}{}
	}

	// The canceled marker goes too, now that the tombstone is what stops a job
	// still waiting in the channel from running. Clearing it here is what stops
	// the marker outliving the worker that would have removed it.
	delete(que.dropped, id)
	delete(que.jobs, id)
}

// Done returns a channel that is closed when the queue is stopped.
func (que *Queue) Done() <-chan struct{} {
	return que.done
}

// GetAllJobs returns every job, newest first.
//
// Jobs with the same CreatedAt are ordered by ID descending.
func (que *Queue) GetAllJobs() []*Job {
	que.mu.RLock()
	defer que.mu.RUnlock()

	result := slices.Collect(maps.Values(que.jobs))
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
func (que *Queue) GetJob(id string) *Job {
	que.mu.RLock()
	defer que.mu.RUnlock()

	return que.jobs[id]
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
func (que *Queue) IfLive(id string, fn func()) {
	que.mu.RLock()
	defer que.mu.RUnlock()

	if _, deleted := que.deleted[id]; deleted {
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
func (que *Queue) Reinstate(job *Job) {
	que.mu.Lock()
	defer que.mu.Unlock()

	// The markers go with it. A worker still unwinding will find no tombstone and
	// record its outcome normally, and one that never started leaves nothing
	// behind to clear a marker.
	delete(que.deleted, job.ID)
	delete(que.dropped, job.ID)

	job.Status = JobStatusCancelled
	job.Error = string(JobStatusCancelled)
	job.UpdatedAt = time.Now()
	que.jobs[job.ID] = job
}

// Restore registers a job without enqueueing it.
func (que *Queue) Restore(job *Job) {
	que.mu.Lock()
	defer que.mu.Unlock()

	que.jobs[job.ID] = job
}

// SetStatusFunc registers a callback invoked on job status changes.
func (que *Queue) SetStatusFunc(fn StatusFunc) {
	que.statusFn = fn
}

// Start starts the job queue workers.
func (que *Queue) Start() {
	for i := range que.workers {
		que.wg.Go(func() {
			que.worker(i)
		})
	}

	logging.Logger.Info().Int("workers", que.workers).Msg("job queue started")
}

// Stop stops the job queue.
func (que *Queue) Stop() {
	que.cancel()
	close(que.jobChan)
	close(que.done)
	que.wg.Wait()
}

// Submit submits a job to the queue.
func (que *Queue) Submit(job *Job) {
	que.mu.Lock()

	que.jobs[job.ID] = job
	que.mu.Unlock()

	que.jobChan <- job

	que.notify(job)

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
func (que *Queue) notify(job *Job) {
	if que.statusFn == nil {
		return
	}

	que.mu.RLock()
	defer que.mu.RUnlock()

	if _, deleted := que.deleted[job.ID]; deleted {
		return
	}

	que.statusFn(job)
}

// processJob processes a single job.
func (que *Queue) processJob(job *Job) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	que.mu.Lock()

	if _, gone := que.deleted[job.ID]; gone {
		// Deleted before it ever started. Both markers go, so neither outlives
		// the worker that would have removed it.
		delete(que.deleted, job.ID)
		delete(que.dropped, job.ID)
		que.mu.Unlock()

		return
	}

	if _, dropped := que.dropped[job.ID]; dropped {
		delete(que.dropped, job.ID)
		que.mu.Unlock()

		return
	}

	job.Status = JobStatusProcessing
	job.UpdatedAt = time.Now()
	que.cancels[job.ID] = cancel
	que.mu.Unlock()

	que.notify(job)

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("processing job")

	err := que.handler(ctx, job)

	result := que.settle(job, err)

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

	que.notify(job)
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
func (que *Queue) settle(job *Job, err error) outcome {
	que.mu.Lock()
	defer que.mu.Unlock()

	_, canceled := que.dropped[job.ID]
	_, gone := que.deleted[job.ID]
	delete(que.dropped, job.ID)
	delete(que.deleted, job.ID)
	delete(que.cancels, job.ID)

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
func (que *Queue) worker(_ int) {
	for {
		select {
		case <-que.done:
			return
		case job, ok := <-que.jobChan:
			if !ok {
				return
			}

			que.processJob(job)
		}
	}
}
