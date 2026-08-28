// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/plex"
)

const (
	testServerName = "Test"
	testServerAddr = "127.0.0.1"
	testToken      = "test-token"
	testScheme     = "https"
)

func TestNewMonitor(t *testing.T) {
	t.Parallel()

	client := plex.NewClient(plex.ClientConfig{
		Product:  "",
		ClientID: "",
		Token:    "",
		Timeout:  0,
		BaseURL:  "",
	})
	server := plex.Server{
		Name:    testServerName,
		Address: testServerAddr,
		Port:    32400,
		Token:   testToken,
		Scheme:  testScheme,
		Local:   false,
	}
	m := NewMonitor(client, server, 10*time.Second)

	assert.Equal(t, 10*time.Second, m.interval)
	assert.Equal(t, server, m.server)
}

func TestMonitor_GetSessions_Empty(t *testing.T) {
	t.Parallel()

	client := plex.NewClient(plex.ClientConfig{
		Product:  "",
		ClientID: "",
		Token:    "",
		Timeout:  0,
		BaseURL:  "",
	})
	server := plex.Server{
		Name:    testServerName,
		Address: testServerAddr,
		Port:    32400,
		Token:   testToken,
		Scheme:  testScheme,
		Local:   false,
	}
	m := NewMonitor(client, server, 10*time.Second)

	sessions := m.GetSessions()
	assert.Empty(t, sessions)
}

func TestMonitor_Stop(t *testing.T) {
	t.Parallel()

	client := plex.NewClient(plex.ClientConfig{
		Product:  "",
		ClientID: "",
		Token:    "",
		Timeout:  0,
		BaseURL:  "",
	})
	server := plex.Server{
		Name:    testServerName,
		Address: testServerAddr,
		Port:    32400,
		Token:   testToken,
		Scheme:  testScheme,
		Local:   false,
	}
	m := NewMonitor(client, server, 100*time.Millisecond)

	m.Start()
	m.Stop()
}
