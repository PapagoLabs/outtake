// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"fmt"
	"time"
)

// previewGate bounds how many preview encodes may run at once.
//
// Previews are rendered inline in the request handler rather than through the
// clip queue, so nothing else limits them. Without a bound, N clicks in quick
// succession start N full encodes of the source at the same time, on top of
// whatever exports the workers are already running.
//
// The gate is deliberately separate from the clip queue. A preview is an
// interactive action the user is waiting on, and queueing it behind a running
// export would make it slower, not faster.
type previewGate struct {
	slots chan struct{}
}

// newPreviewGate returns a gate allowing limit simultaneous previews.
//
// A limit below one is raised to one, so a misconfigured value cannot leave the
// feature permanently unavailable.
//
// Parameters:
//   - limit: Maximum simultaneous previews.
//
// Returns:
//   - gate: The bounded gate.
func newPreviewGate(limit int) *previewGate {
	if limit < 1 {
		limit = 1
	}

	return &previewGate{slots: make(chan struct{}, limit)}
}

// acquire takes a preview slot, waiting up to timeout for one to free.
//
// The wait is bounded and watches the context, so a client that disconnects
// while queued gives its place up rather than holding it for a slot it will
// never use.
//
// Parameters:
//   - ctx: Request context, canceled when the client disconnects.
//   - timeout: How long to wait for a free slot.
//
// Returns:
//   - release: Releases the slot. It is nil unless a slot was taken.
//   - err: Non-nil when no slot became free in time.
func (gate *previewGate) acquire(ctx context.Context, timeout time.Duration) (func(), error) {
	// The common case is a free slot, taken without touching the timer.
	select {
	case gate.slots <- struct{}{}:
		return func() { <-gate.slots }, nil
	default:
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case gate.slots <- struct{}{}:
		return func() { <-gate.slots }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("preview wait: %w", ctx.Err())
	case <-timer.C:
		return nil, errPreviewBusy
	}
}

// capacity reports how many previews may run at once, which the tests use to
// assert the bound was applied from configuration.
func (gate *previewGate) capacity() int {
	return cap(gate.slots)
}
