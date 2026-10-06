// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingSweeper records how often it is swept.
type countingSweeper struct {
	// sweeps counts the calls.
	sweeps atomic.Int32
	// err is what every sweep reports.
	err error
	// cancel ends the sweeper's context after the given number of sweeps.
	cancel context.CancelFunc
	// stopAfter is the sweep count that cancels the context.
	stopAfter int32
}

// errSweepFailed reports a sweep the store could not run.
var errSweepFailed = errors.New("sweep failed")

// Sweep counts the call and cancels the context once enough have run.
//
// Parameters:
//   - _: Unused context.
//
// Returns:
//   - removed: One session per sweep.
//   - err: The configured error.
func (sweeper *countingSweeper) Sweep(context.Context) (int64, error) {
	if sweeper.sweeps.Add(1) >= sweeper.stopAfter {
		sweeper.cancel()
	}

	return 1, sweeper.err
}

func TestSweepSessionsRunsAtStartAndOnEveryTick(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	sweeper := &countingSweeper{cancel: cancel, stopAfter: 3}

	done := make(chan struct{})

	go func() {
		sweepSessions(ctx, sweeper, time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the sweeper did not stop when its context ended")
	}

	assert.Equal(t, int32(3), sweeper.sweeps.Load())
}

func TestSweepSessionsKeepsGoingAfterAFailedSweep(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	sweeper := &countingSweeper{cancel: cancel, stopAfter: 2, err: errSweepFailed}

	sweepSessions(ctx, sweeper, time.Millisecond)

	assert.Equal(t, int32(2), sweeper.sweeps.Load())
}

func TestStartSessionStoreReturnsAStoreOnTheDatabase(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	store := startSessionStore(ctx, db)
	require.NotNil(t, store)

	require.NoError(t, store.Set("sid", []byte("payload"), time.Hour))

	data, err := store.Get("sid")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), data)
}
