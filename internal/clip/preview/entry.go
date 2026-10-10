// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/logging"
)

// View is a read-only snapshot of one preview render.
type View struct {
	// ID identifies the preview.
	ID string
	// Status is where the render has got to.
	Status clip.Status
	// Progress is percent complete, 0 to 100.
	Progress int
	// Error is the plain failure message when the status is failed.
	Error string
	// ErrorDetails is the technical report behind Error, empty when Error
	// says all there is.
	ErrorDetails string
	// Format is what the published file holds, zero until it is read.
	Format clip.Format
}

// entry is one registered preview render.
type entry struct {
	view     View
	cancel   context.CancelFunc
	canceled bool
	created  time.Time
	updated  time.Time
}

// Admission is the outcome of registering a preview render.
type Admission int

// registry tracks preview renders and their progress.
type registry struct {
	mu      sync.Mutex
	entries map[string]*entry
	order   []string
	// evicted holds completed previews retention dropped, whose files are
	// removed once the lock is released.
	evicted []string
	// onEvict removes the file of a completed preview retention dropped.
	onEvict func(previewID string)
	// running counts renders that have not finished.
	running sync.WaitGroup
	// closed refuses new renders once the service is shutting down.
	closed bool
	// describe turns a failed render's error into what the page shows.
	describe clip.DescribeFunc
}

const (
	// AdmittedRender means a new render was started.
	AdmittedRender Admission = iota
	// AdmittedExisting means the id was already rendering and got joined.
	AdmittedExisting
	// RefusedFull means the queue is at its limit and the id is new.
	RefusedFull
	// RefusedClosed means the service is shutting down.
	RefusedClosed
)

const (
	// retained is how many finished previews the registry keeps.
	retained = 64

	// progressDone is the percent a render reports once it finishes.
	progressDone = 100

	// queuedPerSlot is how many previews may be admitted per render slot.
	queuedPerSlot = 4
)

// Done reports whether the render has reached a terminal state and no longer
// needs a slot.
//
// Returns:
//   - finished: True when the render is completed, failed, or canceled.
func (view View) Done() bool {
	return view.Status == clip.StatusCompleted ||
		view.Status == clip.StatusFailed ||
		view.Status == clip.StatusCancelled
}

// newRegistry returns an empty registry.
//
// Parameters:
//   - onEvict: Removes the file of a completed preview retention dropped.
//
// Returns:
//   - registry: The empty registry.
func newRegistry(onEvict func(previewID string)) *registry {
	return &registry{
		mu:       sync.Mutex{},
		entries:  map[string]*entry{},
		order:    nil,
		evicted:  nil,
		onEvict:  onEvict,
		running:  sync.WaitGroup{},
		closed:   false,
		describe: clip.PlainFailure,
	}
}

// add registers a new entry, unless the id is already registered and still
// rendering.
//
// Parameters:
//   - item: The entry to register.
func (registry *registry) add(item *entry) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if registry.retireLocked(item.view.ID) {
		return
	}

	registry.trackLocked(item)
}

// admit registers a render, or reports why it could not.
//
// Parameters:
//   - item: The entry to register.
//   - slots: Number of render slots the gate allows.
//
// Returns:
//   - result: What happened.
func (registry *registry) admit(item *entry, slots int) Admission {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if registry.closed {
		return RefusedClosed
	}

	if registry.retireLocked(item.view.ID) {
		return AdmittedExisting
	}

	outstanding := 0

	for _, existing := range registry.entries {
		if !existing.view.Done() {
			outstanding++
		}
	}

	if outstanding >= slots*queuedPerSlot {
		return RefusedFull
	}

	registry.trackLocked(item)

	// Counted under the lock that also guards closed, so a close that has
	// started waiting never sees a render added behind it.
	registry.running.Add(1)

	return AdmittedRender
}

// cancel stops an in-flight preview.
//
// Parameters:
//   - previewID: Preview id.
//
// Returns:
//   - stopped: True when a running preview was canceled.
func (registry *registry) cancel(previewID string) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.entries[previewID]
	if !ok || job.view.Done() {
		return false
	}

	job.canceled = true
	job.cancel()

	return true
}

// countFinishedLocked reports how many registered previews have finished.
//
// Returns:
//   - count: The number of finished previews.
func (registry *registry) countFinishedLocked() int {
	count := 0

	for _, job := range registry.entries {
		if job.view.Done() {
			count++
		}
	}

	return count
}

// dropLocked removes an id from the order.
//
// Parameters:
//   - previewID: Preview id to remove.
func (registry *registry) dropLocked(previewID string) {
	for i, existing := range registry.order {
		if existing == previewID {
			registry.order = append(registry.order[:i], registry.order[i+1:]...)

			return
		}
	}
}

// evictLocked drops the oldest finished previews beyond the retained count,
// keeping the most recent ones so back-navigation finds a recent result.
func (registry *registry) evictLocked() {
	finished := registry.countFinishedLocked()

	kept := make([]string, 0, len(registry.order))

	for _, previewID := range registry.order {
		if !registry.retainLocked(previewID, &finished) {
			job, ok := registry.entries[previewID]
			if ok && job.view.Status == clip.StatusCompleted {
				registry.evicted = append(registry.evicted, previewID)
			}

			delete(registry.entries, previewID)

			continue
		}

		kept = append(kept, previewID)
	}

	registry.order = kept
}

// finish records how a render ended.
//
// Parameters:
//   - previewID: Preview id.
//   - err: What the render returned, or nil on success.
func (registry *registry) finish(previewID string, err error) {
	defer registry.flushEvicted()

	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.entries[previewID]
	if !ok {
		return
	}

	job.cancel()

	job.updated = time.Now()

	switch {
	case err == nil:
		job.view.Status = clip.StatusCompleted
		job.view.Progress = progressDone
	case job.canceled || errors.Is(err, context.Canceled):
		job.view.Status = clip.StatusCancelled
	default:
		failure := registry.describe("preview "+previewID, err)

		logging.Logger.Warn().
			Str("error", failure.Chain).
			Str("preview_id", previewID).
			Str("ref", failure.Ref).
			Msg("preview failed")

		job.view.Status = clip.StatusFailed
		job.view.Error = failure.Message
		job.view.ErrorDetails = failure.Details
	}

	registry.evictLocked()
}

// flushEvicted removes the files of the completed previews retention dropped.
// It runs without the lock, because removing a file can wait on storage.
func (registry *registry) flushEvicted() {
	registry.mu.Lock()

	evicted := registry.evicted

	registry.evicted = nil
	registry.mu.Unlock()

	for _, previewID := range evicted {
		registry.onEvict(previewID)
	}
}

// remember records an id whose preview is already published, so a client
// polling it finds a terminal status rather than an unknown one.
//
// Parameters:
//   - previewID: Preview id that is already on disk.
//   - format: What the published file holds.
func (registry *registry) remember(previewID string, format clip.Format) {
	defer registry.flushEvicted()

	registry.add(&entry{
		view: View{
			ID:       previewID,
			Status:   clip.StatusCompleted,
			Progress: progressDone,
			Error:    "",
			Format:   format,
		},
		cancel:  func() {},
		created: time.Now(),
		updated: time.Now(),
	})
}

// resolve returns a snapshot of one preview.
//
// Parameters:
//   - previewID: Preview id.
//
// Returns:
//   - view: A copy of the stored state.
//   - ok: False when the id is unknown.
func (registry *registry) resolve(previewID string) (View, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.entries[previewID]
	if !ok {
		return View{}, false
	}

	return job.view, true
}

// retainLocked decides whether one preview stays, consuming a retained slot
// when it does.
//
// Parameters:
//   - previewID: Preview id being considered.
//   - finished: Remaining finished previews allowed to be retained.
//
// Returns:
//   - keep: True when the entry should stay in the order and the map.
func (registry *registry) retainLocked(previewID string, finished *int) bool {
	job, ok := registry.entries[previewID]
	if !ok {
		// Already evicted; drop the stale reference.
		return false
	}

	if !job.view.Done() {
		return true
	}

	if *finished > retained {
		*finished--

		return false
	}

	return true
}

// retireLocked clears a finished entry for an id so the id can be registered
// again, reporting whether the id is already rendering.
//
// Parameters:
//   - previewID: Id being registered.
//
// Returns:
//   - running: True when the id is already rendering.
func (registry *registry) retireLocked(previewID string) bool {
	existing, tracked := registry.entries[previewID]
	if !tracked {
		return false
	}

	if !existing.view.Done() {
		return true
	}

	delete(registry.entries, previewID)
	registry.dropLocked(previewID)

	return false
}

// shutdown refuses new renders, cancels every running one, and waits for them to
// finish or for ctx to end.
//
// Parameters:
//   - ctx: Bounds how long the wait may take.
//
// Returns:
//   - err: The context's error when renders were still running at its end.
func (registry *registry) shutdown(ctx context.Context) error {
	registry.mu.Lock()

	registry.closed = true

	for _, job := range registry.entries {
		if !job.view.Done() {
			job.canceled = true
			job.cancel()
		}
	}

	registry.mu.Unlock()

	finished := make(chan struct{})

	go func() {
		registry.running.Wait()
		close(finished)
	}()

	select {
	case <-finished:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("previews still running: %w", ctx.Err())
	}
}

// trackLocked registers an entry and prunes what retention no longer keeps.
//
// Parameters:
//   - item: The entry to register.
func (registry *registry) trackLocked(item *entry) {
	registry.entries[item.view.ID] = item
	registry.order = append(registry.order, item.view.ID)
	registry.evictLocked()
}
