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

// TestBindingClientWithTokenReadsAsAnotherAccount covers a client for an
// account that does not own the selected server.
//
// It carries that account's token, never the bound one, and there is none
// without a token or a selection.
func TestBindingClientWithTokenReadsAsAnotherAccount(t *testing.T) {
	t.Parallel()

	bind := NewBinding("outtake", "test-client", time.Minute)

	_, _, ok := bind.ClientWithToken("member-token")
	assert.False(t, ok, "nothing is selected yet")

	bind.Set(plex.Server{Name: "server-a", Token: "owner-token"})
	t.Cleanup(bind.Stop)

	client, server, ok := bind.ClientWithToken("member-token")
	require.True(t, ok)
	require.NotNil(t, client)
	assert.Equal(t, "member-token", client.Token)
	assert.Equal(t, "member-token", server.Token)
	assert.Equal(t, "server-a", server.Name)

	_, _, ok = bind.ClientWithToken("")
	assert.False(t, ok, "an account with no token for the server reads nothing")
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

// TestBindingReportsNoRefusedTokenWithoutAServer covers an unbound
// installation.
//
// With no server there is no poll, so no token was refused.
func TestBindingReportsNoRefusedTokenWithoutAServer(t *testing.T) {
	t.Parallel()

	assert.False(t, NewBinding("outtake", "test", time.Second).TokenRejected())
}
