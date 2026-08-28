// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package binding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
)

func TestBinding_SetAndGet(t *testing.T) {
	t.Parallel()

	bind := New("outtake", "test-client", 50*time.Millisecond)
	t.Cleanup(bind.Stop)

	_, ok := bind.Get()
	assert.False(t, ok)

	server := plex.Server{
		Name:    "Living Room",
		Address: "127.0.0.1",
		Port:    32400,
		Token:   "token",
		Scheme:  "http",
		Local:   false,
	}
	bind.Set(server)

	got, ok := bind.Get()
	require.True(t, ok)
	assert.Equal(t, server, got)
	assert.Empty(t, bind.Sessions())
}

func TestBinding_SessionsWithoutServer(t *testing.T) {
	t.Parallel()

	bind := New("outtake", "test-client", time.Second)
	assert.Nil(t, bind.Sessions())
}
