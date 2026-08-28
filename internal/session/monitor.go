// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package session provides session monitoring for Plex servers.
package session

import (
	"context"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex"
)

// Monitor monitors Plex sessions.
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
}

// NewMonitor creates a new session monitor.
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
	}
}

// GetSessions returns the current sessions.
func (mon *Monitor) GetSessions() []plex.Session {
	mon.mu.RLock()
	defer mon.mu.RUnlock()

	result := make([]plex.Session, 0, len(mon.sessions))

	result = append(result, mon.sessions...)

	return result
}

// Start starts the session monitor.
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

// Stop stops the session monitor.
func (mon *Monitor) Stop() {
	mon.once.Do(func() {
		close(mon.stop)
		mon.cancel()
	})
	mon.wg.Wait()
}

// poll polls for session updates.
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

// refresh refreshes the session list.
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

	mon.sessions = sessions
	mon.mu.Unlock()

	logging.Logger.Debug().
		Str("server", mon.server.Name).
		Int("count", len(sessions)).
		Msg("refreshed sessions")
}
