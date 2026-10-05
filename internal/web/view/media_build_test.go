// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

func TestThumbSrcRewritesAPlexPath(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		"/thumbs?path=%2Flibrary%2Fmedia%2F1%2Ffile.jpg",
		ThumbSrc("/library/media/1/file.jpg"),
	)
}

func TestThumbSrcRefusesAPathItCannotProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
	}{
		{name: "no thumb at all", give: ""},
		{name: "a clip thumbnail is served from storage directly", give: "/clip/thumb/123"},
		{name: "a path outside a Plex root", give: "/etc/passwd"},
		{name: "a path that climbs out", give: "/library/../etc/passwd"},
		{name: "a relative path", give: "library/media/1/file.jpg"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, ThumbSrc(test.give))
		})
	}
}

func TestLibraryItems(t *testing.T) {
	t.Parallel()

	items := LibraryItems([]plex.Library{{
		ID:        "2",
		Title:     "TV Shows",
		Type:      "show",
		ThumbPath: "/library/media/1/file.jpg",
	}})

	require.Len(t, items, 1)
	assert.Equal(t, LibraryItem{
		ID:        "2",
		Title:     "TV Shows",
		Type:      "show",
		ThumbPath: "/thumbs?path=%2Flibrary%2Fmedia%2F1%2Ffile.jpg",
	}, items[0])
}

func TestMediaItemsOverridesTheBrowseTrail(t *testing.T) {
	t.Parallel()

	items := MediaItems([]plex.MediaItem{{
		ID:               "10",
		Title:            "Season 3",
		Type:             plex.TypeSeason,
		LibraryID:        "9",
		GrandparentTitle: "Show",
	}}, "2", "9", "Show")

	require.Len(t, items, 1)
	assert.True(t, items[0].Browsable, "a season is something the user can drill into")
	assert.Contains(t, items[0].BrowseURL, "library="+"2",
		"the trail being browsed wins over the library the item is filed under")
}

func TestMediaItemsLabelsAnEpisode(t *testing.T) {
	t.Parallel()

	items := MediaItems([]plex.MediaItem{{
		ID:          "42",
		Title:       "Episode 5",
		Type:        plex.TypeEpisode,
		ParentIndex: 3,
		Index:       5,
	}}, "", "", "")

	require.Len(t, items, 1)
	assert.Equal(t, "S03E05", items[0].EpisodeLabel)
	assert.Equal(t, 3, items[0].ParentIndex)
}

func TestMediaItemsFallsBackToTheItemsOwnLibrary(t *testing.T) {
	t.Parallel()

	items := MediaItems([]plex.MediaItem{{
		ID:        "100",
		Title:     "Movie",
		Type:      "movie",
		LibraryID: "2",
		Year:      1995,
	}}, "", "", "")

	require.Len(t, items, 1)
	assert.Equal(t, 1995, items[0].Year)
	assert.Contains(t, items[0].BrowseURL, "library="+"2")
}

func TestMediaCrumbs(t *testing.T) {
	t.Parallel()

	libs := LibraryItems(
		[]plex.Library{{ID: "2", Title: "TV Shows", Type: "show"}},
	)

	crumbs := MediaCrumbs(libs, "2", "9", "Show", "Season 3")
	require.Len(t, crumbs, 4)
	assert.Equal(t, crumbLibraries, crumbs[0].Title)
	assert.Equal(t, routes.PathMedia, crumbs[0].URL)
	assert.Equal(t, "TV Shows", crumbs[1].Title)
	assert.Equal(t, "Show", crumbs[2].Title)
	assert.Equal(t, "Season 3", crumbs[3].Title)
	assert.Empty(t, crumbs[3].URL, "the container being browsed is not a link")
}

func TestMediaCrumbsAtALibraryRoot(t *testing.T) {
	t.Parallel()

	crumbs := MediaCrumbs(nil, "", "", "", "")
	require.Len(t, crumbs, 1)
	assert.Equal(t, crumbLibraries, crumbs[0].Title)
}

func TestMediaCrumbsFallsBackToTheLibraryID(t *testing.T) {
	t.Parallel()

	crumbs := MediaCrumbs(nil, "77", "", "", "")
	require.Len(t, crumbs, 2)
	assert.Equal(t, "77", crumbs[1].Title,
		"an undiscovered library is still labeled by its id")
}

func TestItemCrumbsEpisodeTrail(t *testing.T) {
	t.Parallel()

	crumbs := ItemCrumbs(plex.MediaItem{
		Title:            "Episode 5",
		Type:             plex.TypeEpisode,
		LibraryID:        "2",
		LibraryTitle:     "TV Shows",
		ParentID:         "10",
		ParentTitle:      "Season 3",
		GrandparentID:    "9",
		GrandparentTitle: "Show",
	}, nil)

	require.Len(t, crumbs, 5)
	assert.Equal(t, crumbLibraries, crumbs[0].Title)
	assert.Equal(t, "TV Shows", crumbs[1].Title)
	assert.Equal(t, "Show", crumbs[2].Title)
	assert.Equal(t, "Season 3", crumbs[3].Title)
	assert.Equal(t, "Episode 5", crumbs[4].Title)
	assert.Empty(t, crumbs[4].URL, "the page you are on is not a link")
}

func TestItemCrumbsSeasonUsesParentShow(t *testing.T) {
	t.Parallel()

	crumbs := ItemCrumbs(plex.MediaItem{
		Title:       "Season 3",
		Type:        plex.TypeSeason,
		LibraryID:   "2",
		ParentID:    "9",
		ParentTitle: "Show",
	}, nil)

	require.Len(t, crumbs, 4)
	assert.Equal(t, "Show", crumbs[2].Title)
	assert.Contains(t, crumbs[2].URL, "parent="+"9")
}

func TestItemCrumbsNamesAnUnnamedSeason(t *testing.T) {
	t.Parallel()

	crumbs := ItemCrumbs(plex.MediaItem{
		Title:         "Episode 5",
		Type:          plex.TypeEpisode,
		LibraryID:     "2",
		ParentID:      "10",
		GrandparentID: "9",
	}, nil)

	require.Len(t, crumbs, 4)
	assert.Equal(t, seasonFallback, crumbs[2].Title)
}

func TestItemCrumbsWithoutALibrary(t *testing.T) {
	t.Parallel()

	crumbs := ItemCrumbs(plex.MediaItem{Title: "Movie", Type: "clip"}, nil)

	require.Len(t, crumbs, 2)
	assert.Equal(t, crumbLibraries, crumbs[0].Title)
	assert.Equal(t, "Movie", crumbs[1].Title)
}

func TestItemCrumbsPrefersTheDiscoveredLibraryTitle(t *testing.T) {
	t.Parallel()

	libs := LibraryItems(
		[]plex.Library{{ID: "2", Title: "TV Shows", Type: "show"}},
	)

	crumbs := ItemCrumbs(plex.MediaItem{
		Title:        "Movie",
		Type:         "movie",
		LibraryID:    "2",
		LibraryTitle: "Stale",
	}, libs)

	require.Len(t, crumbs, 3)
	assert.Equal(t, "TV Shows", crumbs[1].Title,
		"a library discovered on this request is labeled over the title the item carries")
}

func TestItemCrumbsNamesATitlelessLibraryByItsID(t *testing.T) {
	t.Parallel()

	crumbs := ItemCrumbs(plex.MediaItem{
		Title:     "Movie",
		Type:      "movie",
		LibraryID: "2",
	}, nil)

	require.Len(t, crumbs, 3)
	assert.Equal(t, "2", crumbs[1].Title,
		"a library crumb with no title anywhere is still labeled by its id")
	assert.Equal(t, routes.LibraryURL("2"), crumbs[1].URL)
}

func TestSessionTitleParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		give     plex.MediaItem
		want     []Crumb
		wantYear int
	}{
		{
			name: "episode with ids",
			give: plex.MediaItem{
				ID:               "42",
				Title:            "Episode 5",
				Type:             plex.TypeEpisode,
				LibraryID:        "2",
				ParentID:         "10",
				ParentTitle:      "Season 3",
				GrandparentID:    "9",
				GrandparentTitle: "Show",
			},
			want: []Crumb{
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
				{Title: "Episode 5", URL: "/media/item/" + "42"},
			},
		},
		{
			name: "episode missing library",
			give: plex.MediaItem{
				ID:               "42",
				Title:            "Episode 5",
				Type:             plex.TypeEpisode,
				ParentID:         "10",
				ParentTitle:      "Season 3",
				GrandparentID:    "9",
				GrandparentTitle: "Show",
			},
			want: []Crumb{
				{Title: "Show"},
				{Title: "Season 3"},
				{Title: "Episode 5", URL: "/media/item/" + "42"},
			},
		},
		{
			name: "episode missing parent title names the season",
			give: plex.MediaItem{
				ID:               "42",
				Title:            "Episode 5",
				Type:             plex.TypeEpisode,
				LibraryID:        "2",
				ParentID:         "10",
				ParentIndex:      3,
				GrandparentID:    "9",
				GrandparentTitle: "Show",
			},
			want: []Crumb{
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
				{Title: "Episode 5", URL: "/media/item/" + "42"},
			},
		},
		{
			name: "episode with no show at all is one crumb",
			give: plex.MediaItem{
				ID:    "42",
				Title: "Episode 5",
				Type:  plex.TypeEpisode,
			},
			want: []Crumb{{Title: "Episode 5", URL: "/media/item/" + "42"}},
		},
		{
			name: "an episode with neither trail nor title names nothing",
			give: plex.MediaItem{Type: plex.TypeEpisode},
			want: nil,
		},
		{
			name:     "movie with year",
			give:     plex.MediaItem{ID: "100", Title: "Movie", Year: 1995},
			want:     []Crumb{{Title: "Movie", URL: "/media/item/100"}},
			wantYear: 1995,
		},
		{
			name: "movie without year",
			give: plex.MediaItem{ID: "101", Title: "Movie"},
			want: []Crumb{{Title: "Movie", URL: "/media/item/101"}},
		},
		{
			name: "non-episode without id",
			give: plex.MediaItem{Title: "Concert", Type: "clip"},
			want: []Crumb{{Title: "Concert"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, year := SessionTitleParts(test.give)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.wantYear, year)
		})
	}
}

// SessionTitleParts only reaches displayTitleCrumb with an item that has no
// title of its own, so this is the only place its plain-text crumb is reached.
func TestDisplayTitleCrumb(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []Crumb{{Title: "Show · S01E02 · Pilot"}}, displayTitleCrumb(plex.MediaItem{
		Type:             plex.TypeEpisode,
		GrandparentTitle: "Show",
		ParentIndex:      1,
		Index:            2,
		Title:            "Pilot",
	}))
	assert.Nil(t, displayTitleCrumb(plex.MediaItem{}))
}

func TestSeasonLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give plex.MediaItem
		want string
	}{
		{
			name: "a parent title is preferred over the season number",
			give: plex.MediaItem{ParentTitle: "Show", ParentIndex: 3},
			want: "Show",
		},
		{
			name: "a season with no parent title is named by its number",
			give: plex.MediaItem{ParentIndex: 3},
			want: "Season 3",
		},
		{
			name: "a session with neither names nothing",
			give: plex.MediaItem{},
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, SeasonLabel(test.give))
		})
	}
}

func TestSessionBrowseURL(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse(
		SessionBrowseURL("2", "10", "Season 3", "9", "Show"),
	)
	require.NoError(t, err)
	assert.Equal(t, routes.PathMedia, parsed.Path)
	assert.Equal(t, "2", parsed.Query().Get(routes.QueryLibrary))
	assert.Equal(t, "10", parsed.Query().Get(routes.QueryParent))
	assert.Equal(t, "Season 3", parsed.Query().Get(routes.QueryTitle))
	assert.Equal(t, "9", parsed.Query().Get(routes.QueryUp))
	assert.Equal(t, "Show", parsed.Query().Get(routes.QueryUpTitle))
}

func TestSessionBrowseURLWithoutAnIDHasNowhereToLink(t *testing.T) {
	t.Parallel()

	assert.Empty(t, SessionBrowseURL("", "10", "Season 3", "", ""),
		"a browse trail with no library has no page to open")
	assert.Empty(t, SessionBrowseURL("2", "", "Season 3", "", ""),
		"a container with no id has no page to open")
}

func TestSessionItemURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, routes.ItemURL("42", nil), SessionItemURL("42"))
	assert.Equal(t, "/media/item/a%2Fb", SessionItemURL("a/b"),
		"an id that is not safe in a path is escaped")
	assert.Empty(t, SessionItemURL(""), "a session with no item has nowhere to link to")
}
