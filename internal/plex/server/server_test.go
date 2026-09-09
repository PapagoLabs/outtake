// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferUniqueServers(t *testing.T) {
	t.Parallel()

	servers := []Server{
		{Name: "Lounge", Address: "1.2.3.4", Port: 443, Token: "", Scheme: "", Local: false},
		{Name: "Lounge", Address: "192.168.1.5", Port: 32400, Token: "", Scheme: "", Local: true},
		{Name: "Office", Address: "10.0.0.2", Port: 32400, Token: "", Scheme: "", Local: true},
	}

	got := PreferUniqueServers(servers)
	require.Len(t, got, 2)
	assert.Equal(t, "192.168.1.5", got[0].Address)
	assert.Equal(t, "Lounge", got[0].Name)
	assert.Equal(t, "Office", got[1].Name)
}

func TestServerFromURL(t *testing.T) {
	t.Parallel()

	srv, ok := ServerFromURL("http://192.168.1.9:32400", "tok")
	require.True(t, ok)
	assert.Equal(t, "192.168.1.9", srv.Address)
	assert.Equal(t, 32400, srv.Port)
	assert.Equal(t, "http", srv.Scheme)
	assert.Equal(t, "tok", srv.Token)

	_, ok = ServerFromURL("", "tok")
	assert.False(t, ok)

	_, ok = ServerFromURL("http://plex.local", "")
	assert.False(t, ok)
}

func TestSameConnection(t *testing.T) {
	t.Parallel()

	left := Server{Scheme: defaultScheme, Address: "plex.example", Port: 443}
	right := left
	assert.True(t, SameConnection(left, right))

	right.Port = 32400
	assert.False(t, SameConnection(left, right))
}
