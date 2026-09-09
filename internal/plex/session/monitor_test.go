// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package session

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// stubFetcher is a handwritten Fetcher for tests.
type stubFetcher struct {
	mu       sync.Mutex
	sessions []plex.Session
	err      error
}

const (
	testServerName = "Test"
	testServerAddr = "127.0.0.1"
	testToken      = "test-token"
	testScheme     = "https"
	testSessionID  = "sess-1"
	testTitle      = "Movie"
)

var errUnavailable = errors.New("unavailable")

// GetSessionsOnServer implements [Fetcher].
//
// Returns:
//   - items: Result slice; empty when none match.
//   - err: Propagates errors from slices.Clone.
func (stub *stubFetcher) GetSessionsOnServer(
	_ context.Context,
	_ plex.Server,
) ([]plex.Session, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	if stub.err != nil {
		return nil, stub.err
	}

	return slices.Clone(stub.sessions), nil
}

// setError sets the error returned by the next fetch.
//
// Parameters:
//   - err: Error value.
func (stub *stubFetcher) setError(err error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	stub.err = err
}

// newStub returns a Fetcher stub that yields the given sessions.
//
// Parameters:
//   - sessions: Active Plex playback sessions.
//
// Returns:
//   - stubFetcher: A Fetcher stub that yields the given sessions.
func newStub(sessions []plex.Session) *stubFetcher {
	return &stubFetcher{
		mu:       sync.Mutex{},
		sessions: sessions,
		err:      nil,
	}
}

// testServer returns a complete Plex server used by tests.
//
// Returns:
//   - server: A complete Plex server used by tests.
func testServer() plex.Server {
	return plex.Server{
		Name:    testServerName,
		Address: testServerAddr,
		Port:    32400,
		Token:   testToken,
		Scheme:  testScheme,
		Local:   false,
	}
}

// testSession returns a complete playback session used by tests.
//
// Returns:
//   - session: A complete playback session used by tests.
func testSession() plex.Session {
	return plex.Session{
		ID: testSessionID,
		MediaItem: plex.MediaItem{
			ID:           "1",
			Title:        testTitle,
			Type:         "movie",
			Duration:     120,
			ThumbPath:    "",
			LibraryTitle: "",
		},
		Title:      testTitle,
		Duration:   120,
		ViewOffset: 10,
	}
}

func TestNewMonitor(t *testing.T) {
	t.Parallel()

	server := testServer()
	m := NewMonitor(newStub(nil), server, 10*time.Second)

	assert.Equal(t, 10*time.Second, m.interval)
	assert.Equal(t, server, m.server)
}

func TestMonitor_GetSessions_Empty(t *testing.T) {
	t.Parallel()

	m := NewMonitor(newStub(nil), testServer(), 10*time.Second)

	assert.Empty(t, m.GetSessions())
}

func TestMonitor_Stop(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		m := NewMonitor(newStub(nil), testServer(), time.Second)

		m.Start()
		m.Stop()
	})
}

func TestMonitor_RefreshWritesSessions(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		want := testSession()
		m := NewMonitor(newStub([]plex.Session{want}), testServer(), time.Second)

		m.Start()
		t.Cleanup(m.Stop)

		synctest.Wait()

		assert.Equal(t, []plex.Session{want}, m.GetSessions())
	})
}

func TestMonitor_RefreshErrorLeavesCacheUnchanged(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		want := testSession()
		stub := newStub([]plex.Session{want})
		m := NewMonitor(stub, testServer(), time.Second)

		m.Start()
		t.Cleanup(m.Stop)

		synctest.Wait()
		require.Equal(t, []plex.Session{want}, m.GetSessions())

		stub.setError(errUnavailable)

		time.Sleep(time.Second)
		synctest.Wait()

		assert.Equal(t, []plex.Session{want}, m.GetSessions())
	})
}
