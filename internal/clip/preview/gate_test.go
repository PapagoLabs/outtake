// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewGateAdmitsUpToItsLimit(t *testing.T) {
	t.Parallel()

	gate := NewGate(2)
	assert.Equal(t, 2, gate.Capacity())

	first, err := gate.Acquire(t.Context(), time.Second)
	require.NoError(t, err)

	second, err := gate.Acquire(t.Context(), time.Second)
	require.NoError(t, err)

	_, err = gate.Acquire(t.Context(), 20*time.Millisecond)
	require.ErrorIs(t, err, ErrBusy)

	first()
	second()
}

func TestPreviewGateFreesCapacityOnRelease(t *testing.T) {
	t.Parallel()

	gate := NewGate(1)

	held, err := gate.Acquire(t.Context(), time.Second)
	require.NoError(t, err)

	go held()

	released, err := gate.Acquire(t.Context(), time.Second)
	require.NoError(t, err)
	released()
}

func TestPreviewGateReleasesConcurrently(t *testing.T) {
	t.Parallel()

	const (
		limit    = 3
		attempts = 40
	)

	gate := NewGate(limit)

	var (
		mu      sync.Mutex
		running int
		peak    int
		refused []error
		wg      sync.WaitGroup
	)

	for range attempts {
		wg.Go(func() {
			release, err := gate.Acquire(t.Context(), 5*time.Second)
			if err != nil {
				mu.Lock()

				refused = append(refused, err)

				mu.Unlock()

				return
			}

			mu.Lock()

			running++

			if running > peak {
				peak = running
			}

			mu.Unlock()

			time.Sleep(time.Millisecond)

			mu.Lock()

			running--

			mu.Unlock()

			release()
		})
	}

	wg.Wait()

	assert.Empty(t, refused, "every acquire should have been admitted within the wait")
	assert.LessOrEqual(t, peak, limit, "the gate admitted more previews than its limit")
}

func TestPreviewGateStopsWaitingOnCancel(t *testing.T) {
	t.Parallel()

	gate := NewGate(1)

	held, err := gate.Acquire(t.Context(), time.Second)
	require.NoError(t, err)

	defer held()

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() {
		_, waitErr := gate.Acquire(ctx, time.Minute)
		done <- waitErr
	}()

	cancel()

	select {
	case waitErr := <-done:
		require.ErrorIs(t, waitErr, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("acquire did not return after its context was canceled")
	}
}

func TestNewPreviewGateRaisesUnusableLimits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 1, NewGate(0).Capacity())
	assert.Equal(t, 1, NewGate(-4).Capacity())
	assert.Equal(t, 5, NewGate(5).Capacity())
}
