// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/progress"
)

// render registers a preview, runs it in the background, and records how it
// ended.
//
// Parameters:
//   - ctx: Request context supplying values, not cancellation.
//   - previewID: Preview id, which the caller chooses so it can be content
//     addressed or otherwise stable across submissions.
//   - slots: Number of render slots the gate allows.
//   - fn: Renders the preview. Its context carries the progress callback the
//     ffmpeg writer reports into, and is canceled by cancel.
//
// Returns:
//   - result: False when the id is already registered, in which case the
//     existing render is left running and fn is not called. The same preview
//     arriving twice means the same content, so rendering it again is wasted
//     work.
func (registry *registry) render(
	ctx context.Context,
	previewID string,
	slots int,
	fn func(context.Context) error,
) Admission {
	jobCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	item := &entry{
		view: View{
			ID:       previewID,
			Status:   clip.StatusPending,
			Progress: 0,
			Error:    "",
			Format:   clip.Format{},
		},
		cancel:  cancel,
		created: time.Now(),
		updated: time.Now(),
	}

	result := registry.admit(item, slots)
	defer registry.flushEvicted()

	if result != AdmittedRender {
		// Nothing new was registered, so nothing owns this cancel. Releasing it
		// keeps the detached context from being retained. An id that is already
		// rendering must not start a second one over it.
		cancel()

		return result
	}

	go func() {
		defer registry.running.Done()

		registry.setStatus(previewID, clip.StatusProcessing, 0)

		renderCtx := progress.WithProgress(jobCtx, func(percent int) {
			registry.setProgress(previewID, percent)
		})

		err := fn(renderCtx)

		registry.finish(previewID, err)
	}()

	return result
}

// setProgress records how far a render has got, clamped to the reported range.
//
// Parameters:
//   - previewID: Preview id.
//   - percent: Reported completion, which is ignored if it goes backwards.
func (registry *registry) setProgress(previewID string, percent int) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.entries[previewID]
	if !ok {
		return
	}

	if percent < 0 {
		percent = 0
	}

	if percent > progressDone {
		percent = progressDone
	}

	if percent > job.view.Progress {
		job.view.Progress = percent
		job.updated = time.Now()
	}
}

// setFormat records what a published preview holds.
//
// Parameters:
//   - previewID: Preview id.
//   - format: What the published file holds.
func (registry *registry) setFormat(previewID string, format clip.Format) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.entries[previewID]
	if !ok {
		return
	}

	job.view.Format = format
}

// setStatus records where a render has got to.
//
// Parameters:
//   - previewID: Preview id.
//   - status: The new status.
//   - percent: Progress to record alongside it.
func (registry *registry) setStatus(previewID string, status clip.Status, percent int) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	job, ok := registry.entries[previewID]
	if !ok {
		return
	}

	job.view.Status = status
	job.view.Progress = percent
	job.updated = time.Now()
}
