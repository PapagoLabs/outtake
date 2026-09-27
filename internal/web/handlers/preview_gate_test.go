// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/config"
)

// TestPreviewGateAdmitsUpToItsLimit is the core bound: no more than the
// configured number of previews may hold a slot at once.
func TestPreviewGateAdmitsUpToItsLimit(t *testing.T) {
	t.Parallel()

	gate := newPreviewGate(2)
	assert.Equal(t, 2, gate.capacity())

	first, err := gate.acquire(t.Context(), time.Second)
	require.NoError(t, err)

	second, err := gate.acquire(t.Context(), time.Second)
	require.NoError(t, err)

	// A third must not be admitted while the first two are held.
	_, err = gate.acquire(t.Context(), 20*time.Millisecond)
	require.ErrorIs(t, err, errPreviewBusy)

	first()
	second()
}

// TestPreviewGateFreesCapacityOnRelease covers the normal path: releasing lets a
// waiting preview in.
func TestPreviewGateFreesCapacityOnRelease(t *testing.T) {
	t.Parallel()

	gate := newPreviewGate(1)

	held, err := gate.acquire(t.Context(), time.Second)
	require.NoError(t, err)

	go held()

	released, err := gate.acquire(t.Context(), time.Second)
	require.NoError(t, err)
	released()
}

// TestPreviewGateReleasesConcurrently checks the bound under contention, which
// is the case that matters: several clicks arriving together.
func TestPreviewGateReleasesConcurrently(t *testing.T) {
	t.Parallel()

	const (
		limit    = 3
		attempts = 40
	)

	gate := newPreviewGate(limit)

	var (
		mu      sync.Mutex
		running int
		peak    int
		refused []error
		wg      sync.WaitGroup
	)

	for range attempts {
		wg.Go(func() {
			// A refusal is recorded rather than asserted here, because require
			// calls FailNow and that is only legal from the test goroutine.
			release, err := gate.acquire(t.Context(), 5*time.Second)
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

// TestPreviewGateStopsWaitingOnCancel keeps a disconnected client from holding a
// place in the queue.
func TestPreviewGateStopsWaitingOnCancel(t *testing.T) {
	t.Parallel()

	gate := newPreviewGate(1)

	held, err := gate.acquire(t.Context(), time.Second)
	require.NoError(t, err)

	defer held()

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() {
		_, waitErr := gate.acquire(ctx, time.Minute)
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

// TestNewPreviewGateRaisesUnusableLimits covers a misconfigured value, which
// would otherwise leave the feature permanently unavailable.
func TestNewPreviewGateRaisesUnusableLimits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 1, newPreviewGate(0).capacity())
	assert.Equal(t, 1, newPreviewGate(-4).capacity())
	assert.Equal(t, 5, newPreviewGate(5).capacity())
}

// TestClipHandlerTakesPreviewLimitFromConfig checks the gate is sized from
// configuration rather than a constant, so an operator can raise it on a machine
// that has the headroom.
func TestClipHandlerTakesPreviewLimitFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "configured limit is used", limit: 4, want: 4},
		{name: "zero falls back to one", limit: 0, want: 1},
		{name: "negative falls back to one", limit: -2, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler := NewClipHandler(
				nil,
				nil,
				nil,
				&config.Config{MaxConcurrentPreviews: test.limit},
				nil,
				"outtake",
				"test",
			)

			assert.Equal(t, test.want, handler.previews.capacity())
		})
	}
}
