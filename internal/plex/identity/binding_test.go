// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

func TestBindingClientWithoutASelection(t *testing.T) {
	t.Parallel()

	bind := NewBinding("outtake", "test-client", time.Minute)

	client, server, ok := bind.Client()

	assert.False(t, ok, "no server has been selected yet")
	assert.Nil(t, client)
	assert.Empty(t, server.Name)
}

func TestBindingClientCarriesTheSelectedToken(t *testing.T) {
	t.Parallel()

	bind := NewBinding("outtake", "test-client", time.Minute)
	bind.Set(plex.Server{Name: "server-a", Token: "token-a"})

	t.Cleanup(bind.Stop)

	client, server, ok := bind.Client()

	require.True(t, ok)
	assert.NotNil(t, client, "the binding builds a client for the selected server")
	assert.Equal(t, "server-a", server.Name)
	assert.Equal(t, "token-a", server.Token)
}

func TestBindingClientClearedSelection(t *testing.T) {
	t.Parallel()

	store, err := database.New(filepath.Join(t.TempDir(), "binding.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = store.Close() })

	bind := NewBinding("outtake", "test-client", time.Minute)
	bind.Set(plex.Server{Name: "server-a", Token: "token-a"})
	bind.Clear()

	_, _, ok := bind.Client()
	assert.False(t, ok, "a cleared selection has no client to hand out")
}

func TestNewBindingFallsBackToTheDefaultInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
	}{
		{name: "a zero interval falls back", interval: 0},
		{name: "a negative interval falls back", interval: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bind := NewBinding("outtake", "test-client", tt.interval)

			assert.Equal(t, 10*time.Second, bind.interval)
		})
	}
}

func TestBindingSetReplacesTheRunningMonitor(t *testing.T) {
	t.Parallel()

	bind := NewBinding("outtake", "test-client", time.Minute)

	t.Cleanup(bind.Stop)

	bind.Set(plex.Server{Name: "monitor-first", Token: "token-first"})

	first := bind.monitor

	bind.Set(plex.Server{Name: "monitor-second", Token: "token-second"})

	assert.NotSame(t, first, bind.monitor, "the earlier monitor is stopped by the second selection")

	server, ok := bind.Get()
	require.True(t, ok)
	assert.Equal(t, "monitor-second", server.Name)
}

func TestBindingClearWithoutAMonitor(t *testing.T) {
	t.Parallel()

	bind := NewBinding("outtake", "test-client", time.Minute)
	bind.Clear()

	_, ok := bind.Get()
	assert.False(t, ok, "a binding that never monitored has nothing to stop")
}
