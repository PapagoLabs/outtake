// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/library/mocks"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// TestSeesRemembersOnlyADefiniteAnswer covers the cache in front of the
// access check.
//
// A title the server shares, and one it does not, are each asked about once.
// A server that cannot answer reads as not seen and is asked again. The owner
// is never asked about.
func TestSeesRemembersOnlyADefiniteAnswer(t *testing.T) {
	t.Parallel()

	server := sharedPMS(t)
	client := plex.NewClient(
		plex.ClientConfig{Product: "outtake", ClientID: "test", Token: "member-token"},
	)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().ClientWithToken("member-token").Return(client, server, true).Times(4)

	source := NewMediaSource(&config.Config{}, selected, nil)
	member := identity.Viewer{Token: "member-token", PlexID: 7, Owner: false}

	assert.True(t, source.Sees(t.Context(), identity.OwnerViewer(42), "99"))

	for range 2 {
		assert.True(t, source.Sees(t.Context(), member, "42"))
		assert.False(t, source.Sees(t.Context(), member, "99"))
		assert.False(t, source.Sees(t.Context(), member, "500"))
	}
}

// TestSeesAsksAgainOnceAnAnswerExpires covers an answer older than sightTTL,
// so a title Plex stops sharing is hidden again within a few minutes.
func TestSeesAsksAgainOnceAnAnswerExpires(t *testing.T) {
	t.Parallel()

	server := sharedPMS(t)
	client := plex.NewClient(
		plex.ClientConfig{Product: "outtake", ClientID: "test", Token: "member-token"},
	)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().ClientWithToken("member-token").Return(client, server, true).Times(2)

	now := time.Date(2026, time.October, 12, 12, 0, 0, 0, time.UTC)
	source := NewMediaSource(&config.Config{}, selected, nil)

	source.sights = newSightCache(func() time.Time { return now })

	member := identity.Viewer{Token: "member-token", PlexID: 7, Owner: false}

	assert.True(t, source.Sees(t.Context(), member, "42"))

	now = now.Add(sightTTL - time.Second)

	assert.True(t, source.Sees(t.Context(), member, "42"), "the answer is still remembered")

	now = now.Add(time.Second)

	assert.True(t, source.Sees(t.Context(), member, "42"), "the expired answer is asked again")
}

// TestSightCacheStaysWithinItsLimit covers a full cache.
//
// Expired answers go first. When none has expired, every answer goes, so the
// cache never holds more than maxSights.
func TestSightCacheStaysWithinItsLimit(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 12, 12, 0, 0, 0, time.UTC)
	cache := newSightCache(func() time.Time { return now })

	for index := range maxSights - 1 {
		cache.remember(sightKey{token: "t", mediaID: strconv.Itoa(index)}, true)
	}

	now = now.Add(sightTTL)

	cache.remember(sightKey{token: "t", mediaID: "fresh"}, true)
	cache.remember(sightKey{token: "t", mediaID: "next"}, false)

	assert.Len(t, cache.answers, 2, "the expired answers were dropped")

	for index := range maxSights {
		cache.remember(sightKey{token: "u", mediaID: strconv.Itoa(index)}, true)
	}

	assert.LessOrEqual(t, len(cache.answers), maxSights)

	seen, ok := cache.lookup(sightKey{token: "u", mediaID: strconv.Itoa(maxSights - 1)})
	assert.True(t, ok)
	assert.True(t, seen)
}
