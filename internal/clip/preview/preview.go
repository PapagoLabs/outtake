// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

// Service tracks preview renders, bounds how many of them run at once, and owns
// the collaborators a render needs.
type Service struct {
	entries *registry
	gate    *Gate
	store   blob.Blob
	paths   blob.Paths
	ffmpeg  *ffmpeg.ExecFFmpeg
}

// slotWait is how long a preview waits for a free slot before giving up.
const slotWait = 30 * time.Second

// New returns a service whose gate admits limit simultaneous renders and which
// renders through the collaborators it is given.
//
// Parameters:
//   - limit: Maximum simultaneous renders.
//   - store: Destination for a finished preview.
//   - paths: Local layout a preview is staged into and published under.
//   - ffmpeg: Runner used for crop detection and encoding.
//
// Returns:
//   - service: The ready service.
func New(
	limit int,
	store blob.Blob,
	paths blob.Paths,
	runner *ffmpeg.ExecFFmpeg,
) *Service {
	service := &Service{
		entries: nil,
		gate:    NewGate(limit),
		store:   store,
		paths:   paths,
		ffmpeg:  runner,
	}

	service.entries = newRegistry(service.discard)

	return service
}

// Acquire takes a render slot, waiting up to timeout for one to free.
//
// Parameters:
//   - ctx: Request context, canceled when the client disconnects.
//   - timeout: How long to wait for a free slot.
//
// Returns:
//   - release: Releases the slot. It is nil unless a slot was taken.
//   - err: Non-nil when no slot became free in time.
func (service *Service) Acquire(ctx context.Context, timeout time.Duration) (func(), error) {
	release, err := service.gate.Acquire(ctx, timeout)
	if err != nil {
		return nil, fmt.Errorf("acquire preview slot: %w", err)
	}

	return release, nil
}

// Cancel stops an in-flight preview render.
//
// Parameters:
//   - previewID: Preview id.
//
// Returns:
//   - stopped: True when a running preview was canceled.
func (service *Service) Cancel(previewID string) bool {
	return service.entries.cancel(previewID)
}

// Close refuses new previews, cancels the running ones, and waits for them to
// finish, so none is still writing when storage is torn down.
//
// Parameters:
//   - ctx: Bounds how long the wait may take.
//
// Returns:
//   - err: The context's error when previews were still running at its end.
func (service *Service) Close(ctx context.Context) error {
	err := service.entries.shutdown(ctx)
	if err != nil {
		return fmt.Errorf("close previews: %w", err)
	}

	return nil
}

// OutputPath is where a published preview is stored.
//
// Parameters:
//   - previewID: Preview id.
//
// Returns:
//   - path: Local path of the published preview file.
func (service *Service) OutputPath(previewID string) string {
	return service.paths.PreviewPath(previewID)
}

// Published reports whether a preview's file is on disk.
//
// Parameters:
//   - previewID: Preview id.
//
// Returns:
//   - published: True when the published preview file exists.
func (service *Service) Published(previewID string) bool {
	return service.store.FileExists(service.OutputPath(previewID))
}

// Remember records an id whose preview is already published, so a client
// polling it finds a terminal status rather than an unknown one.
//
// Parameters:
//   - previewID: Preview id that is already on disk.
func (service *Service) Remember(previewID string) {
	service.entries.remember(previewID)
}

// RenderInto encodes and publishes a preview, waiting for a slot first.
//
// Parameters:
//   - ctx: Cancellation for the render, held by the registry.
//   - previewID: Id the preview is published under.
//   - source: Resolved source media path.
//   - req: Parsed request carrying the marks and encoding options.
//   - preserveHDR: Whether an HDR source is kept rather than tone mapped.
//
// Returns:
//   - err: Non-nil when no slot freed in time or the preview could not be published.
func (service *Service) RenderInto(
	ctx context.Context,
	previewID, source string,
	req clip.Request,
	preserveHDR bool,
) error {
	release, err := service.Acquire(ctx, slotWait)
	if err != nil {
		return fmt.Errorf("preview slot: %w", err)
	}

	defer release()

	if service.Published(previewID) {
		return nil
	}

	err = Render(
		ctx,
		service.store,
		service.ffmpeg,
		source,
		service.OutputPath(previewID),
		req,
		preserveHDR,
	)
	if err != nil {
		return fmt.Errorf("render preview: %w", err)
	}

	return nil
}

// Slots reports how many renders may run at once, which is also the unit the
// admission limit is measured in.
//
// Returns:
//   - limit: The number of slots the gate admits.
func (service *Service) Slots() int {
	return service.gate.Capacity()
}

// Status returns a snapshot of one preview's render.
//
// Parameters:
//   - previewID: Preview id.
//
// Returns:
//   - view: A copy of the stored state.
//   - ok: False when the id is unknown.
func (service *Service) Status(previewID string) (View, bool) {
	return service.entries.resolve(previewID)
}

// Submit registers a render for an id and runs it in the background, recording
// how it ended.
//
// Parameters:
//   - ctx: Request context supplying values, not cancellation.
//   - previewID: Preview id, which the caller chooses so it can be content
//     addressed or otherwise stable across submissions.
//   - fn: Renders the preview. Its context carries the progress callback the
//     ffmpeg writer reports into, and is canceled by a Cancel.
//
// Returns:
//   - result: What happened. The same preview arriving twice means the same
//     content, so a render already registered is joined rather than run again.
func (service *Service) Submit(
	ctx context.Context,
	previewID string,
	fn func(context.Context) error,
) Admission {
	return service.entries.render(ctx, previewID, service.gate.Capacity(), fn)
}

// discard removes the file of a preview retention dropped, so the preview
// directory holds only the previews the registry still knows.
//
// Parameters:
//   - previewID: Preview id whose file is removed.
func (service *Service) discard(previewID string) {
	err := service.store.DeleteFile(service.OutputPath(previewID))
	if err != nil {
		logging.Logger.Warn().
			Err(err).
			Str("preview_id", previewID).
			Msg("failed to remove an evicted preview")
	}
}
