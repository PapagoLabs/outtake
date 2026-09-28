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

// newPreviewRegistry returns an empty registry.
func newPreviewRegistry() *previewRegistry {
	return &previewRegistry{jobs: map[string]*previewJob{}}
}

// done reports whether the render has reached a terminal state and no longer
// needs a slot.
func (view previewView) done() bool {
	return view.Status == queue.JobStatusCompleted ||
		view.Status == queue.JobStatusFailed ||
		view.Status == queue.JobStatusCancelled
}

// add registers a new job.
//
// Parameters:
//   - job: The job to register.
//
// Returns:
//   - added: False when the id is already registered and still rendering.
func (registry *previewRegistry) add(job *previewJob) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	// A duplicate id is rejected rather than replacing the entry. The id is
	// chosen by the caller and is meant to be stable for the same content, so
	// the same preview arriving twice is expected. Replacing would leave the
	// first render's goroutine writing to the second render's state, and
	// canceling the second render when the first finished.
	if existing, exists := registry.jobs[job.view.ID]; exists {
		// A render still in flight keeps its entry, so a second submission of
		// the same parameters joins it. Replacing would leave the first render's
		// goroutine writing to the second render's state, and canceling the
		// second when the first finished.
		if !existing.view.done() {
			return false
		}

		// The earlier render reached a terminal state without leaving a usable
		// preview, most often because it failed. Registering the id again is
		// what lets it be retried at all: keeping the old entry rejects every
		// later attempt and strands the page on a preview that never arrives.
		delete(registry.jobs, job.view.ID)
		registry.dropLocked(job.view.ID)
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

// countFinishedLocked reports how many registered previews have finished.
//
// The caller must hold the lock.
//
// Returns:
//   - count: The number of finished previews.
func (registry *previewRegistry) countFinishedLocked() int {
	count := 0

	for _, job := range registry.jobs {
		if job.view.done() {
			count++
		}
	}

	return count
}

// dropLocked removes an id from the order.
//
// The caller must hold the lock.
//
// Parameters:
//   - id: Preview id to remove.
func (registry *previewRegistry) dropLocked(id string) {
	for i, existing := range registry.order {
		if existing == id {
			registry.order = append(registry.order[:i], registry.order[i+1:]...)

			return
		}
	}
}

// evictLocked drops the oldest finished previews beyond the retained count,
// keeping the most recent ones so back-navigation finds a recent result.
//
// It scans the whole order rather than stopping at the first render still
// running, so a slow preview cannot hold up cleanup behind it.
//
// The caller must hold the lock.
func (registry *previewRegistry) evictLocked() {
	finished := registry.countFinishedLocked()

	kept := make([]string, 0, len(registry.order))

	for _, id := range registry.order {
		if !registry.retainLocked(id, &finished) {
			delete(registry.jobs, id)

			continue
		}

		kept = append(kept, id)
	}

	registry.order = kept
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

// remember records an id whose preview is already published, so a client
// polling it finds a terminal status rather than an unknown one.
//
// A render already in flight is left alone, so remembering never disturbs a
// render that is still going to overwrite the file.
//
// Parameters:
//   - id: Preview id that is already on disk.
func (registry *previewRegistry) remember(id string) {
	registry.add(&previewJob{
		view: previewView{
			ID:       id,
			Status:   queue.JobStatusCompleted,
			Progress: previewProgressDone,
			Error:    "",
		},
		cancel:  func() {},
		created: time.Now(),
		updated: time.Now(),
	})
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

// retainLocked decides whether one preview stays, consuming a retained slot
// when it does.
//
// A render still running always keeps its place, wherever it sits: a client may
// be polling it or may still cancel it. Stopping at the first one would let a
// single slow preview hold up every eviction behind it, and renders waiting on
// the preview gate are running too, so the map would grow well past the
// retained count.
//
// Finished is decremented when a finished preview is dropped, so the caller can
// walk the order once and keep exactly the most recent previews.
//
// The caller must hold the lock.
//
// Parameters:
//   - id: Preview id being considered.
//   - finished: Remaining finished previews allowed to be retained.
//
// Returns:
//   - keep: True when the entry should stay in the order and the map.
func (registry *previewRegistry) retainLocked(id string, finished *int) bool {
	job, ok := registry.jobs[id]
	if !ok {
		// Already evicted; drop the stale reference.
		return false
	}

	if !job.view.done() {
		return true
	}

	if *finished > previewRetained {
		*finished--

		return false
	}

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
