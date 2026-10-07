// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// answeringConnection starts a server that answers every request and returns
// a connection to it.
//
// Parameters:
//   - t: The test the server belongs to.
//   - local: Whether the connection claims to be local.
//   - relay: Whether the connection claims to be a relay.
//
// Returns:
//   - server: A connection that answers.
func answeringConnection(t *testing.T, local, relay bool) Server {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	host, port := extractAddrPort(t, ts)

	return Server{
		Name:      "Attic",
		Address:   host,
		Port:      port,
		Token:     "srv-token",
		Scheme:    httpScheme,
		Local:     local,
		MachineID: "machine-1",
		Relay:     relay,
	}
}

// deadConnection returns a local connection to a loopback port nothing
// listens on.
//
// Parameters:
//   - t: The test that needs the connection.
//
// Returns:
//   - server: A connection that refuses.
func deadConnection(t *testing.T) Server {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.NoError(t, listener.Close())

	return Server{
		Name:      "Attic",
		Address:   "127.0.0.1",
		Port:      addr.Port,
		Token:     "srv-token",
		Scheme:    httpScheme,
		Local:     true,
		MachineID: "machine-1",
		Relay:     false,
	}
}

// reachClient builds a client with a short timeout for pings.
//
// Returns:
//   - client: The client.
func reachClient() *Client {
	return NewClient(ClientConfig{
		Product:  productName,
		ClientID: "test",
		Token:    "",
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})
}

func TestFirstReachablePrefersALocalConnection(t *testing.T) {
	t.Parallel()

	relay := answeringConnection(t, false, true)
	remote := answeringConnection(t, false, false)
	local := answeringConnection(t, true, false)

	got, ok := reachClient().FirstReachable(t.Context(), []Server{relay, remote, local})

	require.True(t, ok)
	assert.Equal(t, local, got)
}

func TestFirstReachableSkipsAConnectionThatDoesNotAnswer(t *testing.T) {
	t.Parallel()

	relay := answeringConnection(t, false, true)

	got, ok := reachClient().FirstReachable(t.Context(), []Server{deadConnection(t), relay})

	require.True(t, ok)
	assert.Equal(t, relay, got, "the relay is the only connection that answered")
}

func TestFirstReachableReportsNoAnswer(t *testing.T) {
	t.Parallel()

	got, ok := reachClient().FirstReachable(t.Context(), []Server{deadConnection(t), deadConnection(t)})

	assert.False(t, ok)
	assert.Equal(t, EmptyServer(), got)

	_, ok = reachClient().FirstReachable(t.Context(), nil)
	assert.False(t, ok, "a server without connections is unreachable")
}

func TestConnectionRank(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		server Server
		want   int
	}{
		{"local", Server{Local: true, Scheme: httpScheme}, rankLocal},
		{"local relay", Server{Local: true, Relay: true}, rankLocal},
		{"remote https", Server{Scheme: defaultScheme}, rankRemoteHTTPS},
		{"remote http", Server{Scheme: httpScheme}, rankRemote},
		{"relay", Server{Scheme: defaultScheme, Relay: true}, rankRelay},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, connectionRank(test.server), test.name)
	}
}
