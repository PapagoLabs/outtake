// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
)

// sightKey names one account's view of one title.
//
// The account is named by its token for the bound server, which differs for
// each server, so binding another server never reuses an answer.
type sightKey struct {
	// token is the account's token for the bound server.
	token string
	// mediaID is the title's rating key.
	mediaID string
}

// sight is one remembered answer.
type sight struct {
	// expires is when the answer is asked again.
	expires time.Time
	// seen reports that the account sees the title.
	seen bool
}

// sightCache remembers briefly which titles each account sees.
type sightCache struct {
	// now reads the clock.
	now func() time.Time
	// answers holds each remembered answer.
	answers map[sightKey]sight
	// mu guards answers.
	mu sync.Mutex
}

const (
	// sightTTL is how long an answer is remembered.
	//
	// A title Plex stops sharing with an account stays visible to it for at
	// most this long.
	sightTTL = 5 * time.Minute
	// maxSights is how many answers the cache holds before it drops the
	// expired ones.
	maxSights = 4096
)

// Sees reports whether a viewer's Plex account can see a title.
//
// It asks the way CheckAccess does and remembers a definite answer for a few
// minutes. A failure to ask reads as not seen and is not remembered, so the
// next call asks again.
//
// Parameters:
//   - ctx: Request context.
//   - viewer: The account asking.
//   - mediaID: The title's rating key.
//
// Returns:
//   - seen: True for the owner or a title Plex shares with the viewer.
func (source *MediaSource) Sees(ctx context.Context, viewer identity.Viewer, mediaID string) bool {
	if viewer.Owner {
		return true
	}

	key := sightKey{token: viewer.Token, mediaID: mediaID}
	if seen, ok := source.sights.lookup(key); ok {
		return seen
	}

	err := source.CheckAccess(ctx, viewer, mediaID)
	if err != nil && !errors.Is(err, ErrNotShared) {
		return false
	}

	source.sights.remember(key, err == nil)

	return err == nil
}

// newSightCache creates an empty cache.
//
// Parameters:
//   - now: Reads the clock.
//
// Returns:
//   - cache: The cache.
func newSightCache(now func() time.Time) *sightCache {
	return &sightCache{
		now:     now,
		answers: make(map[sightKey]sight),
		mu:      sync.Mutex{},
	}
}

// lookup returns a remembered answer that has not expired.
//
// Parameters:
//   - key: The account and title.
//
// Returns:
//   - seen: The answer.
//   - ok: False when there is none.
//
//nolint:nonamedreturns // The named results tell the answer from its flag.
func (cache *sightCache) lookup(key sightKey) (seen, ok bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	answer, found := cache.answers[key]
	if !found || !cache.now().Before(answer.expires) {
		return false, false
	}

	return answer.seen, true
}

// prune drops the expired answers, or every answer when none has expired.
//
// The caller holds mu.
//
// Parameters:
//   - now: The current time.
func (cache *sightCache) prune(now time.Time) {
	for held, answer := range cache.answers {
		if !now.Before(answer.expires) {
			delete(cache.answers, held)
		}
	}

	if len(cache.answers) >= maxSights {
		clear(cache.answers)
	}
}

// remember keeps an answer for sightTTL.
//
// A full cache drops its expired answers first, and drops every answer when
// none has expired, so it never grows past maxSights.
//
// Parameters:
//   - key: The account and title.
//   - seen: The answer.
func (cache *sightCache) remember(key sightKey, seen bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	now := cache.now()

	if len(cache.answers) >= maxSights {
		cache.prune(now)
	}

	cache.answers[key] = sight{expires: now.Add(sightTTL), seen: seen}
}
