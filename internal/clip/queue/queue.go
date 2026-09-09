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
	mu       sync.RWMutex
	wg       sync.WaitGroup
	handler  JobHandler
	done     chan struct{}
	cancel   context.CancelFunc
	statusFn StatusFunc
}

// StatusFunc is called whenever a job status changes.
type StatusFunc func(job *Job)

const (
	// jobChannelSize is the size of the job channel buffer.
	jobChannelSize = 100

	// progressDone represents 100% progress.
	progressDone = 100
)

// NewQueue creates a new job queue.
//
// Parameters:
//   - workers: Number of concurrent worker goroutines.
//   - handler: Per-job processing callback.
//
// Returns:
//   - queue: A new job queue.
func NewQueue(workers int, handler JobHandler) *Queue {
	_, cancel := context.WithCancel(context.Background())
	queue := &Queue{
		workers:  workers,
		jobChan:  make(chan *Job, jobChannelSize),
		jobs:     make(map[string]*Job),
		cancels:  make(map[string]context.CancelFunc),
		dropped:  make(map[string]struct{}),
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
//   - id: Identifier.
//
// Returns:
//   - ok: True when the condition holds.
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
	job.Error = "canceled"
	job.UpdatedAt = time.Now()
	que.mu.Unlock()

	que.notify(job)

	return true
}

// Delete removes a job from the in-memory map.
//
// Parameters:
//   - id: Identifier.
func (que *Queue) Delete(id string) {
	que.mu.Lock()
	defer que.mu.Unlock()

	if cancel, exists := que.cancels[id]; exists {
		cancel()
		delete(que.cancels, id)
	}

	delete(que.dropped, id)
	delete(que.jobs, id)
}

// Done returns a channel that is closed when the queue is stopped.
//
// Returns:
//   - structType: A channel that is closed when the queue is stopped.
func (que *Queue) Done() <-chan struct{} {
	return que.done
}

// GetAllJobs returns every job, newest first.
//
// Jobs with the same CreatedAt are ordered by ID descending.
//
// Returns:
//   - items: The every job, newest first.
func (que *Queue) GetAllJobs() []*Job {
	que.mu.RLock()
	defer que.mu.RUnlock()

	result := slices.Collect(maps.Values(que.jobs))
	slices.SortFunc(result, compareJobsNewestFirst)

	return result
}

// compareJobsNewestFirst orders jobs by CreatedAt descending, then ID
// descending.
//
// Parameters:
//   - left: Left operand for comparison.
//   - right: Right operand for comparison.
//
// Returns:
//   - n: Numeric result for this call.
func compareJobsNewestFirst(left, right *Job) int {
	if order := right.CreatedAt.Compare(left.CreatedAt); order != 0 {
		return order
	}

	return cmp.Compare(right.ID, left.ID)
}

// GetJob gets a job by ID.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - job: A job by ID.
func (que *Queue) GetJob(id string) *Job {
	que.mu.RLock()
	defer que.mu.RUnlock()

	return que.jobs[id]
}

// Restore registers a job without enqueueing it.
//
// Parameters:
//   - job: Clip job to process or persist.
func (que *Queue) Restore(job *Job) {
	que.mu.Lock()
	defer que.mu.Unlock()

	que.jobs[job.ID] = job
}

// SetStatusFunc registers a callback invoked on job status changes.
//
// Parameters:
//   - fn: Status or progress callback.
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
//
// Parameters:
//   - job: Clip job to process or persist.
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
// Parameters:
//   - job: Clip job to process or persist.
func (que *Queue) notify(job *Job) {
	if que.statusFn != nil {
		que.statusFn(job)
	}
}

// processJob processes a single job.
//
// Parameters:
//   - job: Clip job to process or persist.
func (que *Queue) processJob(job *Job) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	que.mu.Lock()

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

	que.mu.Lock()

	_, canceled := que.dropped[job.ID]
	delete(que.dropped, job.ID)
	delete(que.cancels, job.ID)

	if canceled {
		job.Status = JobStatusCancelled
		job.Error = "canceled"
	} else if err != nil {
		job.Status = JobStatusFailed
		job.Error = err.Error()
	} else {
		job.Status = JobStatusCompleted
		job.Progress = progressDone
	}

	job.UpdatedAt = time.Now()
	que.mu.Unlock()

	if canceled {
		logging.Logger.Info().Str("job_id", job.ID).Msg("job canceled")
	} else if err != nil {
		logging.Logger.Error().
			Str("job_id", job.ID).
			Err(err).
			Msg("job failed")
	} else {
		logging.Logger.Info().
			Str("job_id", job.ID).
			Msg("job completed")
	}

	que.notify(job)
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
