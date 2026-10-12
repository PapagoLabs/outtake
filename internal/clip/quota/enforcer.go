// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package quota

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Store reads the limits and what they count, and records renders.
type Store interface {
	// GlobalLimits returns the limits every member has.
	GlobalLimits(ctx context.Context) (Limits, error)
	// UserLimits returns one member's own limits.
	UserLimits(ctx context.Context, owner int) (Override, error)
	// RendersSince counts a member's renders started since a time, and
	// returns when the oldest of them started.
	RendersSince(ctx context.Context, owner int, since time.Time) (int, time.Time, error)
	// StorageUsed returns how many bytes a member's clips take.
	StorageUsed(ctx context.Context, owner int) (int64, error)
	// RecordRender logs a render a member started.
	RecordRender(ctx context.Context, owner int, at time.Time) error
}

// ActiveFunc counts a member's renders waiting or running, clips and
// previews alike.
type ActiveFunc func(owner int) int

// Member is the account asking for a render.
type Member struct {
	// ID is the account's plex.tv id.
	ID int
	// Exempt is true for the owner and admins, whom no limit binds.
	Exempt bool
}

// Enforcer admits a member's renders while they are within their limits.
type Enforcer struct {
	// store reads the limits and what they count.
	store Store
	// active counts a member's renders waiting or running.
	active ActiveFunc
	// now reads the clock.
	now func() time.Time
	// mu holds each check and the submit it admits together, so two quick
	// requests cannot both pass the same check.
	mu sync.Mutex
}

// New builds an enforcer.
//
// Parameters:
//   - store: Reads the limits and what they count.
//   - active: Counts a member's renders waiting or running.
//
// Returns:
//   - enforcer: An enforcer reading the clock.
func New(store Store, active ActiveFunc) *Enforcer {
	return &Enforcer{store: store, active: active, now: time.Now, mu: sync.Mutex{}}
}

// AdmitClip renders a clip when the member has room for it.
//
// A clip counts toward every limit. It is logged for the weekly limit only
// once its submit succeeds. A nil enforcer admits only an exempt member.
//
// Parameters:
//   - ctx: Request context.
//   - member: The account asking.
//   - submit: Stores and queues the render.
//
// Returns:
//   - err: An *ActiveLimitError, *WeeklyLimitError, or *StorageLimitError
//     for a limit reached, ErrNoLimits without an enforcer, the failure to
//     read the limits, or submit's own failure, each wrapped. A failure to
//     log the render is only logged.
func (enforcer *Enforcer) AdmitClip(ctx context.Context, member Member, submit func() error) error {
	if member.Exempt {
		//nolint:wrapcheck // runSubmit wraps the failure.
		return runSubmit(submit)
	}

	if enforcer == nil {
		return ErrNoLimits
	}

	enforcer.mu.Lock()
	defer enforcer.mu.Unlock()

	err := enforcer.checkClip(ctx, member.ID)
	if err != nil {
		return fmt.Errorf("admit clip: %w", err)
	}

	err = runSubmit(submit)
	if err != nil {
		//nolint:wrapcheck // runSubmit wraps the failure.
		return err
	}

	// The render is queued by now, so a failure to log it only leaves it out
	// of the weekly count, and the request still succeeds.
	err = enforcer.store.RecordRender(ctx, member.ID, enforcer.now())
	if err != nil {
		log.Warn().
			Err(err).
			Int("plex_user_id", member.ID).
			Msg("failed to log a render for the weekly limit")
	}

	return nil
}

// AdmitPreview renders a preview when the member has room for it.
//
// A preview counts toward the concurrent renders and nothing else. A nil
// enforcer admits only an exempt member.
//
// Parameters:
//   - ctx: Request context.
//   - member: The account asking.
//   - submit: Starts the preview's render.
//
// Returns:
//   - err: An *ActiveLimitError, ErrNoLimits without an enforcer, the
//     failure to read the limits, or submit's own failure, each wrapped.
func (enforcer *Enforcer) AdmitPreview(
	ctx context.Context,
	member Member,
	submit func() error,
) error {
	if member.Exempt {
		//nolint:wrapcheck // runSubmit wraps the failure.
		return runSubmit(submit)
	}

	if enforcer == nil {
		return ErrNoLimits
	}

	enforcer.mu.Lock()
	defer enforcer.mu.Unlock()

	limits, err := enforcer.Limits(ctx, member.ID)
	if err != nil {
		return fmt.Errorf("admit preview: %w", err)
	}

	err = enforcer.checkActive(member.ID, limits)
	if err != nil {
		return fmt.Errorf("admit preview: %w", err)
	}

	//nolint:wrapcheck // runSubmit wraps the failure.
	return runSubmit(submit)
}

// ClipCap returns the longest clip a member may make.
//
// Parameters:
//   - ctx: Request context.
//   - member: The account asking.
//   - installation: The installation's own maximum clip length.
//
// Returns:
//   - limit: The member's cap, or the installation's maximum for an exempt
//     member or when the limits cannot be read.
func (enforcer *Enforcer) ClipCap(
	ctx context.Context,
	member Member,
	installation time.Duration,
) time.Duration {
	if member.Exempt || enforcer == nil {
		return installation
	}

	limits, err := enforcer.Limits(ctx, member.ID)
	if err != nil {
		return installation
	}

	return limits.ClipCap(installation)
}

// Limits returns a member's limits, their own laid over the global ones.
//
// Parameters:
//   - ctx: Request context.
//   - owner: The member's plex.tv id.
//
// Returns:
//   - limits: The member's limits.
//   - err: The wrapped failure to read them.
func (enforcer *Enforcer) Limits(ctx context.Context, owner int) (Limits, error) {
	global, err := enforcer.store.GlobalLimits(ctx)
	if err != nil {
		return Limits{}, fmt.Errorf("read global limits: %w", err)
	}

	override, err := enforcer.store.UserLimits(ctx, owner)
	if err != nil {
		return Limits{}, fmt.Errorf("read user limits: %w", err)
	}

	return override.Apply(global), nil
}

// checkActive refuses a render while the member has as many at once as they
// may.
//
// Parameters:
//   - owner: The member's plex.tv id.
//   - limits: The member's limits.
//
// Returns:
//   - err: An *ActiveLimitError, or nil.
func (enforcer *Enforcer) checkActive(owner int, limits Limits) error {
	if limits.Active > 0 && enforcer.active != nil && enforcer.active(owner) >= limits.Active {
		return &ActiveLimitError{Limit: limits.Active}
	}

	return nil
}

// checkClip refuses a clip render that would pass any of the member's limits.
//
// Parameters:
//   - ctx: Request context.
//   - owner: The member's plex.tv id.
//
// Returns:
//   - err: The limit reached, the wrapped failure to read or count, or nil.
func (enforcer *Enforcer) checkClip(ctx context.Context, owner int) error {
	limits, err := enforcer.Limits(ctx, owner)
	if err != nil {
		return fmt.Errorf("read limits: %w", err)
	}

	for _, check := range []func() error{
		func() error { return enforcer.checkActive(owner, limits) },
		func() error { return enforcer.checkWeekly(ctx, owner, limits) },
		func() error { return enforcer.checkStorage(ctx, owner, limits) },
	} {
		err = check()
		if err != nil {
			//nolint:wrapcheck // Each check returns its limit error or a wrapped failure.
			return err
		}
	}

	return nil
}

// checkStorage refuses a clip render while the member's clips take as much
// storage as they may.
//
// Parameters:
//   - ctx: Request context.
//   - owner: The member's plex.tv id.
//   - limits: The member's limits.
//
// Returns:
//   - err: A *StorageLimitError, the wrapped failure to measure, or nil.
func (enforcer *Enforcer) checkStorage(ctx context.Context, owner int, limits Limits) error {
	if limits.Storage <= 0 {
		return nil
	}

	used, err := enforcer.store.StorageUsed(ctx, owner)
	if err != nil {
		return fmt.Errorf("measure storage: %w", err)
	}

	if used >= limits.Storage {
		return &StorageLimitError{Limit: limits.Storage}
	}

	return nil
}

// checkWeekly refuses a clip render once the member has started as many in
// seven days as they may.
//
// Parameters:
//   - ctx: Request context.
//   - owner: The member's plex.tv id.
//   - limits: The member's limits.
//
// Returns:
//   - err: A *WeeklyLimitError, the wrapped failure to count, or nil.
func (enforcer *Enforcer) checkWeekly(ctx context.Context, owner int, limits Limits) error {
	if limits.Weekly <= 0 {
		return nil
	}

	started, oldest, err := enforcer.store.RendersSince(ctx, owner, enforcer.now().Add(-Window))
	if err != nil {
		return fmt.Errorf("count renders: %w", err)
	}

	if started >= limits.Weekly {
		return &WeeklyLimitError{Next: oldest.Add(Window), Limit: limits.Weekly}
	}

	return nil
}

// runSubmit runs a render's submit.
//
// Parameters:
//   - submit: Stores and queues the render, or starts it.
//
// Returns:
//   - err: submit's failure, wrapped.
func runSubmit(submit func() error) error {
	err := submit()
	if err != nil {
		return fmt.Errorf("submit render: %w", err)
	}

	return nil
}
