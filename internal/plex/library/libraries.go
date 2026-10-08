// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// LibraryCache keeps each server's library list for a short while, so the
// sidebar and every browse fragment do not each ask Plex for it. Entries are
// keyed by server, so a server change never reads another server's list.
// Requests that miss together share one Plex request. The zero value is ready
// to use.
type LibraryCache struct {
	fetches singleflight.Group
	entries map[string]libraryEntry
	mu      sync.Mutex
}

// libraryEntry is one server's cached library list.
type libraryEntry struct {
	// libraries is the list Plex returned.
	libraries []plex.Library
	// fetched is when Plex returned it.
	fetched time.Time
}

// libraryTTL is how long a server's library list is reused.
const libraryTTL = time.Minute

// Libraries returns a server's libraries, asking Plex only when the cached
// list is missing or older than libraryTTL. Callers that miss at the same time
// wait on one request, which runs to the client's timeout even when the caller
// that started it goes away. A failed request is not cached.
//
// Parameters:
//   - ctx: Request context. Its end stops this caller waiting.
//   - client: PMS client.
//   - server: PMS to query.
//
// Returns:
//   - libraries: A copy of the server's libraries.
//   - err: Non-nil when Plex had to be asked and could not answer, or ctx
//     ended first.
func (cache *LibraryCache) Libraries(
	ctx context.Context,
	client *plex.Client,
	server plex.Server,
) ([]plex.Library, error) {
	key := plex.SelectionKey(server)

	cache.mu.Lock()

	entry, ok := cache.entries[key]
	cache.mu.Unlock()

	if ok && time.Since(entry.fetched) < libraryTTL {
		return slices.Clone(entry.libraries), nil
	}

	fetch := cache.fetches.DoChan(key, func() (any, error) {
		libraries, err := client.GetLibraries(context.WithoutCancel(ctx), server)
		if err != nil {
			return nil, fmt.Errorf("get libraries: %w", err)
		}

		cache.store(key, libraryEntry{libraries: libraries, fetched: time.Now()})

		return libraries, nil
	})

	select {
	case result := <-fetch:
		if result.Err != nil {
			return nil, result.Err
		}

		//nolint:errcheck,revive // The fetch only ever returns a []plex.Library.
		libraries, _ := result.Val.([]plex.Library)

		return slices.Clone(libraries), nil
	case <-ctx.Done():
		return nil, fmt.Errorf("get libraries: %w", context.Cause(ctx))
	}
}

// store records a server's list unless a newer one is already kept, and drops
// the lists that have expired, so the map holds no more than the servers used
// within the last libraryTTL.
//
// Parameters:
//   - key: The server's cache key.
//   - entry: The list to keep.
func (cache *LibraryCache) store(key string, entry libraryEntry) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.entries == nil {
		cache.entries = map[string]libraryEntry{}
	}

	if kept, ok := cache.entries[key]; ok && kept.fetched.After(entry.fetched) {
		return
	}

	for other, kept := range cache.entries {
		if entry.fetched.Sub(kept.fetched) >= libraryTTL {
			delete(cache.entries, other)
		}
	}

	cache.entries[key] = entry
}
