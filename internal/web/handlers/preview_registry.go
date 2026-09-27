// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/queue"
)

// previewView is a read-only snapshot of one preview render.
//
// Copies are returned rather than the stored job, so a caller reading status
// cannot race with the render writing to it.
type previewView struct {
	// ID identifies the preview.
	ID string
	// Status is where the render has got to.
	Status queue.JobStatus
	// Progress is percent complete, 0 to 100.
	Progress int
	// Error is the failure message when the status is failed.
	Error string
}

// previewJob is one registered preview render.
type previewJob struct {
	view    previewView
	cancel  context.CancelFunc
	created time.Time
	updated time.Time
}

// previewRegistry tracks preview renders and their progress.
//
// It is deliberately not the clip queue. A preview is an interactive action
// whose progress the user is watching, and queueing it behind a running export
// would make it slower rather than faster. Nothing is persisted, because a
// preview is a disposable artifact rather than a clip the user wants to find
// again later.
type previewRegistry struct {
	mu    sync.Mutex
	jobs  map[string]*previewJob
	order []string
}

const (
	// PreviewRetained is how many finished previews the registry keeps so a
	// client polling straight after submitting can still read the result.
	//
	// Bounded because nothing else removes them: a preview is an in-memory
	// render, and the files it writes are reclaimed separately.
	previewRetained = 64

	// PreviewProgressDone is the percent reported once a render finishes.
	previewProgressDone = 100
)

// done reports whether the render has reached a terminal state and no longer
// needs a slot.
func (view previewView) done() bool {
	return view.Status == queue.JobStatusCompleted ||
		view.Status == queue.JobStatusFailed ||
		view.Status == queue.JobStatusCancelled
}

// newPreviewRegistry returns an empty registry.
func newPreviewRegistry() *previewRegistry {
	return &previewRegistry{jobs: map[string]*previewJob{}}
}

// add stores a new job, evicting the oldest finished one if needed.
//
// Parameters:
//   - job: The job to register.
//
// Returns:
//   - added: False when the id is already registered.
func (registry *previewRegistry) add(job *previewJob) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	// A duplicate id is rejected rather than replacing the entry. The id is
	// chosen by the caller and is meant to be stable for the same content, so
	// the same preview arriving twice is expected. Replacing would leave the
	// first render's goroutine writing to the second render's state, and
	// canceling the second render when the first finished.
	if _, exists := registry.jobs[job.view.ID]; exists {
		return false
	}

	registry.jobs[job.view.ID] = job
	registry.order = append(registry.order, job.view.ID)
	registry.evictLocked()

	return true
}

// cancel stops an in-flight preview.
//
// Parameters:
//   - id: Preview id.
//
// Returns:
//   - stopped: True when a running preview was canceled.
func (registry *previewRegistry) cancel(id string) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.jobs[id]
	if !ok || job.view.done() {
		return false
	}

	job.cancel()

	return true
}

// evictLocked drops the oldest finished previews beyond the retained count.
//
// The caller must hold the lock.
func (registry *previewRegistry) evictLocked() {
	for len(registry.order) > previewRetained {
		oldest := registry.order[0]

		job, ok := registry.jobs[oldest]
		if !ok {
			// Already gone; drop the stale reference and carry on.
			registry.order = registry.order[1:]

			continue
		}

		// A render still in flight keeps its place at the front. Retention is a
		// ring, so nothing behind it may be evicted ahead of it, and popping it
		// here would leave the entry in the map with nothing left to track it,
		// so it could never be reclaimed. This cannot stall eviction
		// indefinitely: previews in flight are separately bounded by the
		// preview gate, which caps the map at retained plus running.
		if !job.view.done() {
			return
		}

		registry.order = registry.order[1:]

		delete(registry.jobs, oldest)
	}
}

// finish records how a render ended.
//
// Parameters:
//   - id: Preview id.
//   - err: What the render returned, or nil on success.
func (registry *previewRegistry) finish(id string, err error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.jobs[id]
	if !ok {
		return
	}

	job.cancel()

	job.updated = time.Now()

	switch {
	case err == nil:
		job.view.Status = queue.JobStatusCompleted
		job.view.Progress = previewProgressDone
	case errors.Is(err, context.Canceled):
		job.view.Status = queue.JobStatusCancelled
	default:
		job.view.Status = queue.JobStatusFailed
		job.view.Error = err.Error()
	}

	registry.evictLocked()
}

// get returns a snapshot of one preview.
//
// Parameters:
//   - id: Preview id.
//
// Returns:
//   - view: A copy of the stored state.
//   - ok: False when the id is unknown.
func (registry *previewRegistry) get(id string) (previewView, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.jobs[id]
	if !ok {
		return previewView{}, false
	}

	return job.view, true
}

// render registers a preview, runs it in the background, and records how it
// ended.
//
// The render is detached from ctx so it outlives the request that asked for it,
// which is the point of rendering a preview asynchronously. It still carries
// ctx's values.
//
// Parameters:
//   - ctx: Request context supplying values, not cancellation.
//   - id: Preview id, which the caller chooses so it can be content addressed
//     or otherwise stable across submissions.
//   - fn: Renders the preview. Its context carries the progress callback the
//     ffmpeg writer reports into, and is canceled by cancel.
//
// Returns:
//   - started: False when the id is already registered, in which case the
//     existing render is left running and fn is not called. The same preview
//     arriving twice means the same content, so rendering it again is wasted
//     work and would fight the first render for the entry.
func (registry *previewRegistry) render(
	ctx context.Context,
	id string,
	fn func(context.Context) error,
) bool {
	jobCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	job := &previewJob{
		view: previewView{
			ID:       id,
			Status:   queue.JobStatusPending,
			Progress: 0,
			Error:    "",
		},
		cancel:  cancel,
		created: time.Now(),
		updated: time.Now(),
	}

	if !registry.add(job) {
		// Nothing was registered, so nothing owns this cancel. Releasing it
		// keeps the detached context from being retained.
		cancel()

		return false
	}

	go func() {
		registry.setStatus(id, queue.JobStatusProcessing, 0)

		renderCtx := media.WithProgress(jobCtx, func(percent int) {
			registry.setProgress(id, percent)
		})

		err := fn(renderCtx)

		registry.finish(id, err)
	}()

	return true
}

// setProgress records how far a render has got, clamped to the reported range.
//
// Parameters:
//   - id: Preview id.
//   - percent: Reported completion, which is ignored if it goes backwards.
func (registry *previewRegistry) setProgress(id string, percent int) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.jobs[id]
	if !ok {
		return
	}

	if percent < 0 {
		percent = 0
	}

	if percent > previewProgressDone {
		percent = previewProgressDone
	}

	if percent > job.view.Progress {
		job.view.Progress = percent
		job.updated = time.Now()
	}
}

// setStatus records where a render has got to.
//
// Parameters:
//   - id: Preview id.
//   - status: The new status.
//   - progress: Progress to record alongside it.
func (registry *previewRegistry) setStatus(id string, status queue.JobStatus, progress int) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.jobs[id]
	if !ok {
		return
	}

	job.view.Status = status
	job.view.Progress = progress
	job.updated = time.Now()
}
