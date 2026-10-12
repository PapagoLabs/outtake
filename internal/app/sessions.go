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
// The caller waits on swept before it closes the database. A sweep still
// running when the database closes holds its connection until the query ends,
// and SQLite then touches the database's files after the close has returned.
//
// Parameters:
//   - ctx: Lifetime context the sweeper runs under.
//   - db: Database the sessions live in.
//
// Returns:
//   - store: The session store the router persists sessions through.
//   - swept: Closed once the sweeper has stopped, after ctx ends.
func startSessionStore(
	ctx context.Context,
	db *database.DB,
) (*database.SessionStore, <-chan struct{}) {
	store := database.NewSessionStore(db)
	swept := make(chan struct{})

	go func() {
		defer close(swept)

		sweepSessions(ctx, store, sessionSweepInterval)
	}()

	return store, swept
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
			// A select picks at random when the context ends as a tick is
			// due, so the context is checked again before sweeping.
			if ctx.Err() != nil {
				return
			}
		}
	}
}

// sweepOnce deletes expired sessions and logs the outcome.
//
// A failure caused by ctx ending is not logged.
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
