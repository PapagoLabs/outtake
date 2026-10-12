// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package quota

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore answers the enforcer from fixed values and records renders.
type fakeStore struct {
	// oldest is when the oldest counted render started.
	oldest time.Time
	// err fails every read.
	err error
	// recordErr fails every render log.
	recordErr error
	// override is the member's own limits.
	override Override
	// recorded are the renders logged.
	recorded []time.Time
	// global is the limits every member has.
	global Limits
	// used is the member's storage in bytes.
	used int64
	// started is the member's renders in the window.
	started int
}

// errStoreDown stands in for a store that cannot be read.
var errStoreDown = errors.New("store down")

// errSubmitFailed stands in for a render that could not be queued.
var errSubmitFailed = errors.New("submit failed")

// member is the account the tests render as.
var member = Member{ID: 7, Exempt: false}

// GlobalLimits returns the fake's global limits.
//
// Returns:
//   - limits: The global limits.
//   - err: The fake's err.
func (store *fakeStore) GlobalLimits(context.Context) (Limits, error) {
	return store.global, store.err
}

// RecordRender logs a render.
//
// Parameters:
//   - at: When it started.
//
// Returns:
//   - err: The fake's recordErr.
func (store *fakeStore) RecordRender(_ context.Context, _ int, at time.Time) error {
	store.recorded = append(store.recorded, at)

	return store.recordErr
}

// RendersSince returns the fake's count.
//
// Returns:
//   - started: The renders in the window.
//   - oldest: When the oldest started.
//   - err: Always nil.
func (store *fakeStore) RendersSince(context.Context, int, time.Time) (int, time.Time, error) {
	return store.started, store.oldest, nil
}

// StorageUsed returns the fake's storage.
//
// Returns:
//   - used: The member's bytes.
//   - err: Always nil.
func (store *fakeStore) StorageUsed(context.Context, int) (int64, error) {
	return store.used, nil
}

// UserLimits returns the fake's override.
//
// Returns:
//   - override: The member's limits.
//   - err: Always nil.
func (store *fakeStore) UserLimits(context.Context, int) (Override, error) {
	return store.override, nil
}

// limitOf finds the limit error in a chain.
//
// Parameters:
//   - err: The error.
//
// Returns:
//   - limit: The limit error, or err itself when it holds none.
func limitOf(err error) error {
	if active, ok := errors.AsType[*ActiveLimitError](err); ok {
		return active
	}

	if weekly, ok := errors.AsType[*WeeklyLimitError](err); ok {
		return weekly
	}

	if storage, ok := errors.AsType[*StorageLimitError](err); ok {
		return storage
	}

	return err
}

// TestAdmitClipKeepsAMemberWithinEachLimit covers every limit a clip counts
// toward.
//
// A member at a limit is refused with that limit, and nothing is submitted or
// logged. A member within every limit renders, and the render is logged.
func TestAdmitClipKeepsAMemberWithinEachLimit(t *testing.T) {
	t.Parallel()

	oldest := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)

	for name, test := range map[string]struct {
		store  fakeStore
		active int
		want   error
	}{
		"within":  {store: fakeStore{global: Defaults(), started: 3, used: 10}, active: 1, want: nil},
		"active":  {store: fakeStore{global: Defaults()}, active: 2, want: &ActiveLimitError{Limit: 2}},
		"weekly":  {store: fakeStore{global: Defaults(), started: 100, oldest: oldest}, want: &WeeklyLimitError{Next: oldest.Add(Window), Limit: 100}},
		"storage": {store: fakeStore{global: Defaults(), used: DefaultStorage}, want: &StorageLimitError{Limit: DefaultStorage}},
		"no limits": {
			store: fakeStore{global: Limits{}, started: 1000, used: DefaultStorage * 10}, active: 50, want: nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store := test.store
			enforcer := New(&store, func(int) int { return test.active })
			submitted := false

			err := enforcer.AdmitClip(t.Context(), member, func() error {
				submitted = true

				return nil
			})

			if test.want == nil {
				require.NoError(t, err)
				assert.True(t, submitted)
				assert.Len(t, store.recorded, 1, "the render is logged")

				return
			}

			assert.Equal(t, test.want, limitOf(err))
			assert.False(t, submitted)
			assert.Empty(t, store.recorded)
		})
	}
}

// TestAdmitClipFollowsTheMembersOwnLimits covers a member whose limits an
// admin raised above the global ones.
func TestAdmitClipFollowsTheMembersOwnLimits(t *testing.T) {
	t.Parallel()

	more := 5
	store := fakeStore{global: Defaults(), override: Override{Active: &more}}
	enforcer := New(&store, func(int) int { return 4 })

	require.NoError(t, enforcer.AdmitClip(t.Context(), member, func() error { return nil }))
}

// TestAdmitClipLogsOnlyARenderThatWasQueued covers a submit that fails.
//
// Its error comes back, and nothing is logged against the weekly limit.
func TestAdmitClipLogsOnlyARenderThatWasQueued(t *testing.T) {
	t.Parallel()

	store := fakeStore{global: Defaults()}

	err := New(&store, nil).AdmitClip(t.Context(), member, func() error { return errSubmitFailed })

	require.ErrorIs(t, err, errSubmitFailed)
	assert.Empty(t, store.recorded)
}

// TestAdmitClipRefusesWhenTheLimitsCannotBeRead covers a store that fails,
// which refuses the render rather than letting it through unbounded.
func TestAdmitClipRefusesWhenTheLimitsCannotBeRead(t *testing.T) {
	t.Parallel()

	store := fakeStore{err: errStoreDown}

	err := New(&store, nil).AdmitClip(t.Context(), member, func() error { return nil })

	require.ErrorIs(t, err, errStoreDown)
}

// TestAdmitExemptsTheOwnerAndAdmins covers an exempt account, which renders
// past every limit, even with no enforcer wired, and is never logged.
func TestAdmitExemptsTheOwnerAndAdmins(t *testing.T) {
	t.Parallel()

	exempt := Member{ID: 42, Exempt: true}
	store := fakeStore{global: Limits{Active: 1, Weekly: 1, Storage: 1}, started: 9, used: 9}
	enforcer := New(&store, func(int) int { return 9 })

	require.NoError(t, enforcer.AdmitClip(t.Context(), exempt, func() error { return nil }))
	require.NoError(t, enforcer.AdmitPreview(t.Context(), exempt, func() error { return nil }))
	assert.Empty(t, store.recorded)

	var none *Enforcer

	require.NoError(t, none.AdmitClip(t.Context(), exempt, func() error { return nil }))
}

// TestAdmitRefusesAMemberWithoutAnEnforcer covers missing wiring, which
// never leaves a member unbounded.
func TestAdmitRefusesAMemberWithoutAnEnforcer(t *testing.T) {
	t.Parallel()

	var none *Enforcer

	require.ErrorIs(
		t,
		none.AdmitClip(t.Context(), member, func() error { return nil }),
		ErrNoLimits,
	)
	require.ErrorIs(
		t,
		none.AdmitPreview(t.Context(), member, func() error { return nil }),
		ErrNoLimits,
	)
}

// TestAdmitPreviewCountsOnlyRendersAtOnce covers a preview, which the weekly
// and storage limits leave alone.
func TestAdmitPreviewCountsOnlyRendersAtOnce(t *testing.T) {
	t.Parallel()

	store := fakeStore{global: Defaults(), started: 100, used: DefaultStorage}

	require.NoError(t, New(&store, func(int) int { return 1 }).
		AdmitPreview(t.Context(), member, func() error { return nil }))
	assert.Equal(
		t,
		&ActiveLimitError{Limit: DefaultActive},
		limitOf(New(&store, func(int) int { return 2 }).
			AdmitPreview(t.Context(), member, func() error { return nil })),
	)
	assert.Empty(t, store.recorded, "a preview is not logged against the week")
}

// TestClipCapTakesTheShorterLimit covers the longest clip a member may make.
func TestClipCapTakesTheShorterLimit(t *testing.T) {
	t.Parallel()

	installation := 10 * time.Minute
	short := time.Minute
	long := time.Hour

	for name, test := range map[string]struct {
		override Override
		want     time.Duration
	}{
		"global":        {override: Override{}, want: installation},
		"shorter":       {override: Override{ClipLength: &short}, want: short},
		"longer capped": {override: Override{ClipLength: &long}, want: installation},
	} {
		store := fakeStore{global: Defaults(), override: test.override}

		assert.Equal(
			t,
			test.want,
			New(&store, nil).ClipCap(t.Context(), member, installation),
			name,
		)
	}

	var none *Enforcer

	assert.Equal(t, installation, none.ClipCap(t.Context(), member, installation))
}

// TestLimitErrorsNameTheLimit covers the refusals' error text.
func TestLimitErrorsNameTheLimit(t *testing.T) {
	t.Parallel()

	assert.Contains(t, (&ActiveLimitError{Limit: 2}).Error(), "2")
	assert.Contains(t, (&WeeklyLimitError{Limit: 100}).Error(), "100")
	assert.Contains(t, (&StorageLimitError{Limit: 5}).Error(), "5 bytes")
}

// TestIsLimitFindsAReachedLimit covers telling a reached limit from any other
// failure, anywhere in the chain.
func TestIsLimitFindsAReachedLimit(t *testing.T) {
	t.Parallel()

	assert.True(t, IsLimit(fmt.Errorf("admit: %w", &ActiveLimitError{Limit: 1})))
	assert.True(t, IsLimit(&WeeklyLimitError{Limit: 1}))
	assert.True(t, IsLimit(&StorageLimitError{Limit: 1}))
	assert.False(t, IsLimit(ErrNoLimits))
	assert.False(t, IsLimit(errSubmitFailed))
}

// TestAdmitClipSucceedsWhenTheRenderCannotBeLogged covers a render queued
// before its log failed, which still succeeds, since refusing it now would
// lead to a second clip on a retry.
func TestAdmitClipSucceedsWhenTheRenderCannotBeLogged(t *testing.T) {
	t.Parallel()

	store := fakeStore{global: Defaults(), recordErr: errStoreDown}

	require.NoError(t, New(&store, nil).AdmitClip(t.Context(), member, func() error { return nil }))
}
