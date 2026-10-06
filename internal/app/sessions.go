// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/store/database"
)

// sessionSweeper deletes expired sessions.
type sessionSweeper interface {
	// Sweep deletes expired sessions and reports how many it removed.
	Sweep(ctx context.Context) (int64, error)
}

// sessionSweepInterval is how often expired sessions are deleted.
const sessionSweepInterval = 10 * time.Minute

// startSessionStore creates the database session store and deletes expired
// sessions until ctx ends.
//
// Parameters:
//   - ctx: Lifetime context the sweeper runs under.
//   - db: Database the sessions live in.
//
// Returns:
//   - store: The session store the router persists sessions through.
func startSessionStore(ctx context.Context, db *database.DB) *database.SessionStore {
	store := database.NewSessionStore(db)

	go sweepSessions(ctx, store, sessionSweepInterval)

	return store
}

// sweepSessions deletes expired sessions once at start and then on every
// tick, until ctx ends.
//
// Parameters:
//   - ctx: Lifetime context the sweeper runs under.
//   - sessions: Store to sweep.
//   - interval: Time between sweeps.
func sweepSessions(ctx context.Context, sessions sessionSweeper, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		sweepOnce(ctx, sessions)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sweepOnce deletes expired sessions and logs the outcome. A failure caused by
// ctx ending is not logged.
//
// Parameters:
//   - ctx: Lifetime context the sweep runs under.
//   - sessions: Store to sweep.
func sweepOnce(ctx context.Context, sessions sessionSweeper) {
	removed, err := sessions.Sweep(ctx)
	if err != nil && ctx.Err() == nil {
		log.Warn().Err(err).Msg("failed to delete expired sessions")

		return
	}

	if removed > 0 {
		log.Debug().Int64("removed", removed).Msg("deleted expired sessions")
	}
}
