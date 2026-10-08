// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// countingPMS is a Plex server that counts library list requests and fails
// them while failing is set.
type countingPMS struct {
	// server is the PMS a client queries.
	server plex.Server
	// calls counts the library list requests.
	calls atomic.Int32
	// release lets held requests answer once it is closed.
	release chan struct{}
	// failing makes the library list answer with a server error.
	failing atomic.Bool
	// held makes the library list wait for release before answering.
	held atomic.Bool
}

// librariesBody is a server's library list as Plex reports it.
const librariesBody = `{"MediaContainer":{"Directory":[
	{"key":"1","title":"Movies","type":"movie"},
	{"key":"2","title":"TV Shows","type":"show"}
]}}`

// newCountingPMS starts a counting Plex server.
//
// Parameters:
//   - t: The test the server belongs to.
//
// Returns:
//   - pms: The running server.
func newCountingPMS(t *testing.T) *countingPMS {
	t.Helper()

	pms := &countingPMS{release: make(chan struct{})}

	ts := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/library/sections/all" {
				writer.WriteHeader(http.StatusNotFound)

				return
			}

			pms.calls.Add(1)

			if pms.held.Load() {
				<-pms.release
			}

			if pms.failing.Load() {
				writer.WriteHeader(http.StatusInternalServerError)

				return
			}

			writer.Header().Set("Content-Type", "application/json")

			_, _ = writer.Write([]byte(librariesBody))
		}),
	)
	t.Cleanup(ts.Close)
	t.Cleanup(func() {
		if pms.held.Load() {
			select {
			case <-pms.release:
			default:
				close(pms.release)
			}
		}
	})

	parsed, err := url.Parse(ts.URL)
	require.NoError(t, err)

	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)

	pms.server = plex.Server{
		Name:      "counting",
		Address:   parsed.Hostname(),
		Port:      port,
		Token:     "token",
		Scheme:    "http",
		Local:     true,
		MachineID: "counting-" + parsed.Port(),
		Relay:     false,
	}

	return pms
}

// librariesClient builds a Plex client for the counting servers.
//
// Returns:
//   - client: The PMS client.
func librariesClient() *plex.Client {
	return plex.NewClient(plex.ClientConfig{
		Product:  "outtake",
		ClientID: "test",
		Token:    "token",
		Timeout:  5 * time.Second,
	})
}

// TestLibraryCacheAsksPlexOncePerServer covers several pages asking for the
// same server's libraries within the cache window: Plex answers once.
func TestLibraryCacheAsksPlexOncePerServer(t *testing.T) {
	t.Parallel()

	pms := newCountingPMS(t)
	client := librariesClient()

	var cache LibraryCache

	for range 3 {
		libraries, err := cache.Libraries(t.Context(), client, pms.server)
		require.NoError(t, err)
		assert.Len(t, libraries, 2)
	}

	assert.Equal(t, int32(1), pms.calls.Load())
}

// TestLibraryCacheKeepsServersApart covers two servers sharing one cache:
// each is asked for its own list.
func TestLibraryCacheKeepsServersApart(t *testing.T) {
	t.Parallel()

	first := newCountingPMS(t)
	second := newCountingPMS(t)
	client := librariesClient()

	var cache LibraryCache

	for _, pms := range []*countingPMS{first, second, first, second} {
		_, err := cache.Libraries(t.Context(), client, pms.server)
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), first.calls.Load())
	assert.Equal(t, int32(1), second.calls.Load())
}

// TestLibraryCacheDoesNotKeepAFailure covers a failed request: the next page
// asks Plex again rather than reading the failure back.
func TestLibraryCacheDoesNotKeepAFailure(t *testing.T) {
	t.Parallel()

	pms := newCountingPMS(t)
	client := librariesClient()

	var cache LibraryCache

	pms.failing.Store(true)

	_, err := cache.Libraries(t.Context(), client, pms.server)
	require.Error(t, err)

	pms.failing.Store(false)

	libraries, err := cache.Libraries(t.Context(), client, pms.server)
	require.NoError(t, err)
	assert.Len(t, libraries, 2)
	assert.Equal(t, int32(2), pms.calls.Load())
}

// TestLibraryCacheAsksAgainOnceTheListExpires covers an entry older than
// libraryTTL: it is fetched again, and expired entries are dropped.
func TestLibraryCacheAsksAgainOnceTheListExpires(t *testing.T) {
	t.Parallel()

	pms := newCountingPMS(t)
	stale := newCountingPMS(t)
	client := librariesClient()

	cache := LibraryCache{
		entries: map[string]libraryEntry{
			plex.SelectionKey(pms.server): {
				libraries: []plex.Library{{ID: "9", Title: "Old"}},
				fetched:   time.Now().Add(-2 * libraryTTL),
			},
			plex.SelectionKey(stale.server): {
				libraries: nil,
				fetched:   time.Now().Add(-2 * libraryTTL),
			},
		},
	}

	libraries, err := cache.Libraries(t.Context(), client, pms.server)
	require.NoError(t, err)

	assert.Len(t, libraries, 2, "the expired list is replaced by what Plex reports")
	assert.Equal(t, int32(1), pms.calls.Load())
	assert.NotContains(t, cache.entries, plex.SelectionKey(stale.server),
		"an expired entry for another server is dropped")
}

// TestLibraryCacheHandsOutCopies covers a caller changing the list it was
// given: the cached list is untouched.
func TestLibraryCacheHandsOutCopies(t *testing.T) {
	t.Parallel()

	pms := newCountingPMS(t)
	client := librariesClient()

	var cache LibraryCache

	first, err := cache.Libraries(t.Context(), client, pms.server)
	require.NoError(t, err)

	first[0].Title = "Changed"

	second, err := cache.Libraries(t.Context(), client, pms.server)
	require.NoError(t, err)
	assert.Equal(t, "Movies", second[0].Title)
}

// TestLibraryCacheSharesOneRequestBetweenConcurrentMisses covers the sidebar
// and a browse fragment missing together: Plex is asked once and every caller
// gets the list.
func TestLibraryCacheSharesOneRequestBetweenConcurrentMisses(t *testing.T) {
	t.Parallel()

	pms := newCountingPMS(t)
	pms.held.Store(true)

	client := librariesClient()

	var (
		cache LibraryCache
		group sync.WaitGroup
	)

	const callers = 5

	type answer struct {
		err   error
		count int
	}

	answers := make(chan answer, callers)

	for range callers {
		group.Go(func() {
			libraries, err := cache.Libraries(t.Context(), client, pms.server)

			answers <- answer{err: err, count: len(libraries)}
		})
	}

	require.Eventually(t, func() bool { return pms.calls.Load() == 1 },
		5*time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	close(pms.release)
	group.Wait()
	close(answers)

	for got := range answers {
		require.NoError(t, got.err)
		assert.Equal(t, 2, got.count)
	}

	assert.Equal(t, int32(1), pms.calls.Load())
}

// TestLibraryCacheStopsWaitingWhenTheCallerLeaves covers a caller whose
// request ends while Plex is still answering: it returns at once, and the
// shared request still fills the cache for the next caller.
func TestLibraryCacheStopsWaitingWhenTheCallerLeaves(t *testing.T) {
	t.Parallel()

	pms := newCountingPMS(t)
	pms.held.Store(true)

	client := librariesClient()

	var cache LibraryCache

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := cache.Libraries(ctx, client, pms.server)
	require.ErrorIs(t, err, context.Canceled)

	require.Eventually(t, func() bool { return pms.calls.Load() == 1 },
		5*time.Second, time.Millisecond)
	close(pms.release)

	require.Eventually(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()

		_, ok := cache.entries[plex.SelectionKey(pms.server)]

		return ok
	}, 5*time.Second, time.Millisecond)

	libraries, err := cache.Libraries(t.Context(), client, pms.server)
	require.NoError(t, err)
	assert.Len(t, libraries, 2)
	assert.Equal(t, int32(1), pms.calls.Load())
}

// TestLibraryCacheKeepsTheNewerList covers a slow request finishing after a
// newer one: the older list does not replace the newer.
func TestLibraryCacheKeepsTheNewerList(t *testing.T) {
	t.Parallel()

	now := time.Now()

	var cache LibraryCache

	cache.store("server", libraryEntry{libraries: []plex.Library{{ID: "new"}}, fetched: now})
	cache.store("server", libraryEntry{
		libraries: []plex.Library{{ID: "old"}},
		fetched:   now.Add(-time.Second),
	})

	assert.Equal(t, "new", cache.entries["server"].libraries[0].ID)
}
