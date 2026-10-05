// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/session"
)

// Binding holds the process-wide Plex server selection and live sessions.
type Binding struct {
	product  string
	clientID string
	interval time.Duration

	mu       sync.RWMutex
	selected plex.Server
	ok       bool
	monitor  *session.Monitor
}

// NewBinding creates a binding that polls sessions at the given interval.
//
// Parameters:
//   - product: Plex product name sent with API requests.
//   - clientID: Plex client identifier.
//   - interval: Session poll interval.
//
// Returns:
//   - binding: A ready-to-use server binding.
func NewBinding(product, clientID string, interval time.Duration) *Binding {
	const defaultInterval = 10 * time.Second

	if interval <= 0 {
		interval = defaultInterval
	}

	return &Binding{
		product:  product,
		clientID: clientID,
		interval: interval,
		mu:       sync.RWMutex{},
		selected: plex.EmptyServer(),
		ok:       false,
		monitor:  nil,
	}
}

// Clear drops the selected server and stops session monitoring.
func (bind *Binding) Clear() {
	bind.mu.Lock()
	defer bind.mu.Unlock()

	if bind.monitor != nil {
		bind.monitor.Stop()

		bind.monitor = nil
	}

	bind.selected = plex.EmptyServer()
	bind.ok = false
}

// Client returns a Plex client for the currently selected server.
//
// Returns:
//   - client: A Plex client scoped to the selected server.
//   - server: The selected server.
//   - ok: False when no server has been selected yet.
func (bind *Binding) Client() (*plex.Client, plex.Server, bool) {
	server, ok := bind.Get()
	if !ok {
		return nil, plex.EmptyServer(), false
	}

	return plex.NewClient(plex.ClientConfig{
		Product:  bind.product,
		ClientID: bind.clientID,
		Token:    server.Token,
	}), server, true
}

// Get returns the selected server.
//
// Returns:
//   - server: The currently selected Plex server.
//   - ok: True when a server has been selected.
func (bind *Binding) Get() (plex.Server, bool) {
	bind.mu.RLock()
	defer bind.mu.RUnlock()

	return bind.selected, bind.ok
}

// Sessions returns the latest cached playback sessions.
//
// Returns:
//   - sessions: A copy of the current session list.
func (bind *Binding) Sessions() []plex.Session {
	bind.mu.RLock()

	mon := bind.monitor
	bind.mu.RUnlock()

	if mon == nil {
		return nil
	}

	return mon.GetSessions()
}

// Set selects a Plex server and starts session monitoring.
//
// Parameters:
//   - selected: The Plex Media Server to bind to.
func (bind *Binding) Set(selected plex.Server) {
	bind.mu.Lock()
	defer bind.mu.Unlock()

	if bind.monitor != nil {
		bind.monitor.Stop()
	}

	client := plex.NewClient(plex.ClientConfig{
		Product:  bind.product,
		ClientID: bind.clientID,
		Token:    selected.Token,
		Timeout:  0,
		BaseURL:  "",
	})

	bind.selected = selected
	bind.ok = true
	bind.monitor = session.NewMonitor(client, selected, bind.interval)
	bind.monitor.Start()
}

// Stop ends session monitoring.
func (bind *Binding) Stop() {
	bind.Clear()
}
