// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package session

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex"
)

// Monitor polls a Plex Media Server for live playback sessions.
type Monitor struct {
	client   *plex.Client
	server   plex.Server
	interval time.Duration

	mu       sync.RWMutex
	wg       sync.WaitGroup
	sessions []plex.Session

	stop   chan struct{}
	once   sync.Once
	cancel context.CancelFunc

	// failure is the last poll error that was logged, empty after a success.
	// Only the poll goroutine reads or writes it.
	failure string
	// unauthorized reports that the last poll was refused for its token.
	unauthorized atomic.Bool
}

// NewMonitor creates a session monitor that polls client at interval.
//
// Parameters:
//   - client: Plex client used to query the server.
//   - server: Plex Media Server to poll.
//   - interval: Time between session refreshes.
//
// Returns:
//   - monitor: A monitor that has not been started.
func NewMonitor(client *plex.Client, server plex.Server, interval time.Duration) *Monitor {
	return &Monitor{
		client:   client,
		server:   server,
		interval: interval,
		mu:       sync.RWMutex{},
		wg:       sync.WaitGroup{},
		sessions: nil,
		stop:     make(chan struct{}),
		once:     sync.Once{},
		cancel:   func() {},
		failure:  "",
	}
}

// GetSessions returns a copy of the cached playback sessions.
//
// Returns:
//   - sessions: A clone of the current session list.
func (mon *Monitor) GetSessions() []plex.Session {
	mon.mu.RLock()
	defer mon.mu.RUnlock()

	return slices.Clone(mon.sessions)
}

// Start begins polling for session updates.
func (mon *Monitor) Start() {
	logging.Logger.Info().
		Str("server", mon.server.Name).
		Dur("interval", mon.interval).
		Msg("starting session monitor")

	ctx, cancel := context.WithCancel(context.Background())

	mon.cancel = cancel
	mon.wg.Go(func() {
		mon.poll(ctx)
	})
}

// Stop ends polling and waits for the poll goroutine to exit.
func (mon *Monitor) Stop() {
	mon.once.Do(func() {
		close(mon.stop)
		mon.cancel()
	})
	mon.wg.Wait()
}

// Unauthorized reports whether the server refused the last poll's token.
//
// Returns:
//   - refused: True until a poll succeeds again.
func (mon *Monitor) Unauthorized() bool {
	return mon.unauthorized.Load()
}

// poll polls for session updates until the monitor is stopped.
//
// Parameters:
//   - ctx: Parent context canceled when the monitor stops.
func (mon *Monitor) poll(ctx context.Context) {
	ticker := time.NewTicker(mon.interval)
	defer ticker.Stop()

	mon.refresh(ctx)

	for {
		select {
		case <-mon.stop:
			return
		case <-ticker.C:
			mon.refresh(ctx)
		}
	}
}

// refresh fetches sessions and replaces the cache on success.
//
// Parameters:
//   - ctx: Parent context for the session fetch timeout.
func (mon *Monitor) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, mon.interval)
	defer cancel()

	sessions, err := mon.client.GetSessionsOnServer(ctx, mon.server)
	if err != nil {
		mon.reportFailure(err)

		return
	}

	mon.reportRecovery()

	mon.mu.Lock()

	mon.sessions = slices.Clone(sessions)
	mon.mu.Unlock()

	logging.Logger.Debug().
		Str("server", mon.server.Name).
		Int("count", len(sessions)).
		Msg("refreshed sessions")
}

// reportFailure records a failed poll. A failure is logged as a warning once,
// and repeats of the same failure are logged at debug, so a server that keeps
// refusing does not fill the log every poll.
//
// Parameters:
//   - err: Why the poll failed.
func (mon *Monitor) reportFailure(err error) {
	mon.unauthorized.Store(errors.Is(err, plex.ErrUnauthorized))

	message := err.Error()
	if message == mon.failure {
		logging.Logger.Debug().
			Err(err).
			Str("server", mon.server.Name).
			Msg("still failing to fetch sessions")

		return
	}

	mon.failure = message

	logging.Logger.Warn().
		Err(err).
		Str("server", mon.server.Name).
		Msg("failed to fetch sessions")
}

// reportRecovery records a successful poll, noting when it ends a failure.
func (mon *Monitor) reportRecovery() {
	mon.unauthorized.Store(false)

	if mon.failure == "" {
		return
	}

	mon.failure = ""

	logging.Logger.Info().
		Str("server", mon.server.Name).
		Msg("fetching sessions works again")
}
