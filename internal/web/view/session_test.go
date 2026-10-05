// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

func TestSessionItemsMapsEveryField(t *testing.T) {
	t.Parallel()

	items := SessionItems([]plex.Session{{
		ID:    "session-1",
		Title: "Show",
		MediaItem: plex.MediaItem{
			ID:               "42",
			Title:            "Episode 5",
			Type:             plex.TypeEpisode,
			LibraryID:        "2",
			ParentID:         "10",
			ParentTitle:      "Season 3",
			GrandparentID:    "9",
			GrandparentTitle: "Show",
		},
		ViewOffset: 90.5,
		Duration:   1500.25,
	}})

	require.Len(t, items, 1)
	assert.Equal(t, SessionItem{
		ID:      "session-1",
		MediaID: "42",
		Parts: []Crumb{
			{
				Title: "Show",
				URL:   SessionBrowseURL("2", "9", "Show", "", ""),
			},
			{
				Title: "Season 3",
				URL: SessionBrowseURL(
					"2",
					"10",
					"Season 3",
					"9",
					"Show",
				),
			},
			{Title: "Episode 5", URL: SessionItemURL("42")},
		},
		ViewOffset: 90500 * time.Millisecond,
		Duration:   1500250 * time.Millisecond,
	}, items[0])
}

func TestSessionItemsCarriesTheReleaseYear(t *testing.T) {
	t.Parallel()

	items := SessionItems([]plex.Session{{
		ID:         "session-1",
		MediaItem:  plex.MediaItem{ID: "100", Title: "Movie", Year: 1995},
		ViewOffset: 90.5,
	}})

	require.Len(t, items, 1)
	assert.Equal(t, 1995, items[0].Year,
		"a movie carries its release year, an episode carries none")
	assert.Equal(t, []Crumb{{Title: "Movie", URL: SessionItemURL("100")}}, items[0].Parts)
}

func TestSessionItemsKeepTheOrderPlexGave(t *testing.T) {
	t.Parallel()

	items := SessionItems([]plex.Session{
		{ID: "session-1", MediaItem: plex.MediaItem{ID: "100"}},
		{ID: "session-2", MediaItem: plex.MediaItem{ID: "101"}},
	})

	require.Len(t, items, 2)
	assert.Equal(t, "session-1", items[0].ID)
	assert.Equal(t, "session-2", items[1].ID)
}

func TestSessionItemsAreEmptyForNoSessions(t *testing.T) {
	t.Parallel()

	assert.Empty(t, SessionItems(nil))
}

func TestSessionItemResumeURLCarriesThePlaybackOffset(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse(
		SessionItem{MediaID: "42", ViewOffset: 90500 * time.Millisecond}.ResumeURL(),
	)
	require.NoError(t, err)

	assert.Equal(t, routes.PathItemPrefix+"42", parsed.Path)
	assert.Equal(t, "90.500", parsed.Query().Get(routes.QueryStart),
		"the Clip now link opens the item at the moment playback stopped")
}

func TestSessionItemResumeURLWithoutAMediaIDHasNowhereToLink(t *testing.T) {
	t.Parallel()

	assert.Empty(t, SessionItem{ViewOffset: 90500 * time.Millisecond}.ResumeURL(),
		"a session playing something without an id has no item page to resume")
}
