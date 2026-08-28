// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package queue provides a simple job queue for processing media jobs.
package queue

import (
	"context"
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
	mu       sync.RWMutex
	handler  JobHandler
	done     chan struct{}
	cancel   context.CancelFunc
	statusFn StatusFunc
}

// StatusFunc is called whenever a job status changes.
type StatusFunc func(job *Job)

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
		mu:       sync.RWMutex{},
		handler:  handler,
		done:     make(chan struct{}),
		cancel:   cancel,
		statusFn: nil,
	}

	return queue
}

// Delete removes a job from the in-memory map.
func (que *Queue) Delete(id string) {
	que.mu.Lock()
	defer que.mu.Unlock()

	delete(que.jobs, id)
}

// Done returns a channel that is closed when the queue is stopped.
func (que *Queue) Done() <-chan struct{} {
	return que.done
}

// GetAllJobs returns all jobs.
func (que *Queue) GetAllJobs() []*Job {
	que.mu.RLock()
	defer que.mu.RUnlock()

	result := make([]*Job, 0, len(que.jobs))
	for _, job := range que.jobs {
		result = append(result, job)
	}

	return result
}

// GetJob gets a job by ID.
func (que *Queue) GetJob(id string) *Job {
	que.mu.RLock()
	defer que.mu.RUnlock()

	return que.jobs[id]
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
		go que.worker(i)
	}

	logging.Logger.Info().Int("workers", que.workers).Msg("job queue started")
}

// Stop stops the job queue.
func (que *Queue) Stop() {
	que.cancel()
	close(que.jobChan)
	close(que.done)
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
func (que *Queue) notify(job *Job) {
	if que.statusFn != nil {
		que.statusFn(job)
	}
}

// processJob processes a single job.
func (que *Queue) processJob(job *Job) {
	job.Status = JobStatusProcessing
	job.UpdatedAt = time.Now()
	que.notify(job)

	logging.Logger.Info().
		Str("job_id", job.ID).
		Str("type", string(job.Type)).
		Msg("processing job")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := que.handler(ctx, job)
	if err != nil {
		job.Status = JobStatusFailed
		job.Error = err.Error()
		logging.Logger.Error().
			Str("job_id", job.ID).
			Err(err).
			Msg("job failed")
	} else {
		job.Status = JobStatusCompleted
		job.Progress = progressDone
		logging.Logger.Info().
			Str("job_id", job.ID).
			Msg("job completed")
	}

	job.UpdatedAt = time.Now()
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
