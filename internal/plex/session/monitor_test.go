// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package session

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
)

type sessionServer struct {
	server *httptest.Server
	body   string

	mu       sync.Mutex
	requests int
	failing  bool
}

func newSessionServer(t *testing.T) *sessionServer {
	t.Helper()

	const testSessionsJSON = `{"MediaContainer":{"size":1,"Metadata":[
	{"ratingKey":"1","title":"Movie","type":"movie","duration":120000,"viewOffset":10000,
	 "Session":{"id":"sess-1"}}
]}}`

	fake := &sessionServer{
		server:   nil,
		body:     testSessionsJSON,
		mu:       sync.Mutex{},
		requests: 0,
		failing:  false,
	}

	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)

	return fake
}

func (fake *sessionServer) count() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return fake.requests
}

func (fake *sessionServer) fail() {
	fake.mu.Lock()

	fake.failing = true

	fake.mu.Unlock()
}

func (fake *sessionServer) pms(t *testing.T) plex.Server {
	t.Helper()

	parsed, ok := plex.ServerFromURL(fake.server.URL, "test-token")
	require.True(t, ok, "the test server URL must parse into a Plex server")

	return parsed
}

func (fake *sessionServer) serve(writer http.ResponseWriter, _ *http.Request) {
	fake.mu.Lock()

	fake.requests++

	failing := fake.failing
	body := fake.body

	fake.mu.Unlock()

	if failing {
		writer.WriteHeader(http.StatusInternalServerError)

		return
	}

	writer.Header().Set("Content-Type", "application/json")

	_, _ = writer.Write([]byte(body))
}

func newTestMonitor(t *testing.T, fake *sessionServer) (*Monitor, plex.Server) {
	t.Helper()

	server := fake.pms(t)

	monitor := NewMonitor(
		plex.NewClient(plex.ClientConfig{
			BaseURL: fake.server.URL,
			Timeout: 5 * time.Second,
		}),
		server,
		10*time.Millisecond,
	)
	t.Cleanup(monitor.Stop)

	return monitor, server
}

func TestNewMonitor(t *testing.T) {
	t.Parallel()

	m, server := newTestMonitor(t, newSessionServer(t))

	assert.Equal(t, 10*time.Millisecond, m.interval)
	assert.Equal(t, server, m.server)
}

func TestMonitor_GetSessions_Empty(t *testing.T) {
	t.Parallel()

	m, _ := newTestMonitor(t, newSessionServer(t))

	assert.Empty(t, m.GetSessions())
}

func TestMonitor_Stop(t *testing.T) {
	t.Parallel()

	m, _ := newTestMonitor(t, newSessionServer(t))

	m.Start()
	m.Stop()
}

func TestMonitor_RefreshWritesSessions(t *testing.T) {
	t.Parallel()

	m, _ := newTestMonitor(t, newSessionServer(t))

	m.Start()

	require.Eventually(t, func() bool {
		return len(m.GetSessions()) == 1
	}, 5*time.Second, time.Millisecond, "the first refresh must publish the playing session")

	sessions := m.GetSessions()
	assert.Equal(t, "sess-1", sessions[0].ID)
	assert.Equal(t, "Movie", sessions[0].Title)
	assert.InEpsilon(t, 120.0, sessions[0].Duration, 0.01)
	assert.InEpsilon(t, 10.0, sessions[0].ViewOffset, 0.01)
}

func TestMonitor_RefreshErrorLeavesCacheUnchanged(t *testing.T) {
	t.Parallel()

	fake := newSessionServer(t)

	m, _ := newTestMonitor(t, fake)

	m.Start()

	require.Eventually(t, func() bool {
		return len(m.GetSessions()) == 1
	}, 5*time.Second, time.Millisecond, "the first refresh must publish the playing session")

	cached := m.GetSessions()

	fake.fail()

	require.Eventually(t, func() bool {
		return fake.count() >= 2
	}, 5*time.Second, time.Millisecond, "a later refresh must reach the failing server")

	assert.Equal(t, cached, m.GetSessions(),
		"a refresh that failed must leave the cache exactly as it was")
}
