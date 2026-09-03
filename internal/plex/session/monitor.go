// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package session

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex"
)

// Fetcher loads live playback sessions from a Plex Media Server.
type Fetcher interface {
	// GetSessionsOnServer returns the sessions currently playing on server.
	//
	// Parameters:
	//   - ctx: Cancellation and deadline for the fetch.
	//   - server: The Plex Media Server to query.
	//
	// Returns:
	//   - sessions: The active playback sessions.
	//   - err: Non-nil when the server cannot be queried.
	GetSessionsOnServer(ctx context.Context, server plex.Server) ([]plex.Session, error)
}

// Monitor polls a Plex Media Server for live playback sessions.
type Monitor struct {
	client   Fetcher
	server   plex.Server
	interval time.Duration

	mu       sync.RWMutex
	wg       sync.WaitGroup
	sessions []plex.Session

	stop   chan struct{}
	once   sync.Once
	cancel context.CancelFunc
}

// *plex.Client satisfies Fetcher.
var _ Fetcher = (*plex.Client)(nil)

// NewMonitor creates a session monitor that polls client at interval.
//
// The returned monitor is not started. Call [Monitor.Start] to begin polling.
//
// Parameters:
//   - client: Session fetcher, typically a [*plex.Client].
//   - server: Plex Media Server to poll.
//   - interval: Time between session refreshes.
//
// Returns:
//   - monitor: A monitor that has not been started.
func NewMonitor(client Fetcher, server plex.Server, interval time.Duration) *Monitor {
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
// A fetch error leaves the cache unchanged.
//
// Parameters:
//   - ctx: Parent context for the session fetch timeout.
func (mon *Monitor) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, mon.interval)
	defer cancel()

	sessions, err := mon.client.GetSessionsOnServer(ctx, mon.server)
	if err != nil {
		logging.Logger.Warn().
			Err(err).
			Str("server", mon.server.Name).
			Msg("failed to fetch sessions")

		return
	}

	mon.mu.Lock()

	mon.sessions = slices.Clone(sessions)
	mon.mu.Unlock()

	logging.Logger.Debug().
		Str("server", mon.server.Name).
		Int("count", len(sessions)).
		Msg("refreshed sessions")
}
