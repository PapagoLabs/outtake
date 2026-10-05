// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Gate bounds how many preview encodes may run at once.
type Gate struct {
	slots chan struct{}
}

// ErrBusy is reported when no slot frees within the wait a caller asked for.
var ErrBusy = errors.New("too many previews are already rendering")

// NewGate returns a gate allowing limit simultaneous renders.
//
// Parameters:
//   - limit: Maximum simultaneous renders.
//
// Returns:
//   - gate: The bounded gate.
func NewGate(limit int) *Gate {
	if limit < 1 {
		limit = 1
	}

	return &Gate{slots: make(chan struct{}, limit)}
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
func (gate *Gate) Acquire(ctx context.Context, timeout time.Duration) (func(), error) {
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
		return nil, ErrBusy
	}
}

// Capacity reports how many renders may run at once.
//
// Returns:
//   - limit: The number of slots the gate admits.
func (gate *Gate) Capacity() int {
	return cap(gate.slots)
}
