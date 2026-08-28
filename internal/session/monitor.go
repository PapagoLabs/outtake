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
	sessions []plex.Session

	stop chan struct{}
	once sync.Once
}

// NewMonitor creates a new session monitor.
func NewMonitor(client *plex.Client, server plex.Server, interval time.Duration) *Monitor {
	return &Monitor{
		client:   client,
		server:   server,
		interval: interval,
		mu:       sync.RWMutex{},
		sessions: nil,
		stop:     make(chan struct{}),
		once:     sync.Once{},
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

	go mon.poll()
}

// Stop stops the session monitor.
func (mon *Monitor) Stop() {
	mon.once.Do(func() {
		close(mon.stop)
	})
}

// poll polls for session updates.
func (mon *Monitor) poll() {
	ticker := time.NewTicker(mon.interval)
	defer ticker.Stop()

	mon.refresh()

	for {
		select {
		case <-mon.stop:
			return
		case <-ticker.C:
			mon.refresh()
		}
	}
}

// refresh refreshes the session list.
func (mon *Monitor) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), mon.interval)
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
