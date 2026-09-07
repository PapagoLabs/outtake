// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package browse

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestMediaResultsEmptyLibraryHidesChooser(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaResults(view.MediaProps{
		Items: nil,
		Libraries: []view.LibraryItem{
			{ID: "1", Title: "Movies", Type: "movie"},
		},
		Crumbs: []view.Crumb{
			{Title: "Libraries", URL: "/media"},
			{Title: "Movies", URL: "/media?library=1"},
		},
		Query:     "",
		LibraryID: "1",
		HasServer: true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "This folder is empty.")
	assert.NotContains(t, body, ">Browse<")
}

func TestMediaResultsEmptyFolderHidesChooser(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaResults(view.MediaProps{
		Items: nil,
		Libraries: []view.LibraryItem{
			{ID: "1", Title: "Movies", Type: "movie"},
		},
		Crumbs: []view.Crumb{
			{Title: "Libraries", URL: "/media"},
			{Title: "Movies", URL: "/media?library=1"},
			{Title: "Show", URL: "/media?library=1&parent=2"},
		},
		Query:     "",
		LibraryID: "1",
		HasServer: true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "This folder is empty.")
	assert.Contains(t, body, "Show")
	assert.NotContains(t, body, ">Browse<")
}

func TestMediaBrowseLibraryRoot(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaBrowse(view.MediaProps{
		Items: []view.MediaItem{
			{ID: "10", Title: "Heat", Type: "movie"},
		},
		LibraryID: "1",
		Sort:      "title_asc",
		Letters: []view.LetterIndex{
			{Title: "#", Size: 3, Start: 0},
			{Title: "A", Size: 40, Start: 3},
			{Title: "H", Size: 8, Start: 43},
		},
		Letter:    "H",
		Start:     43,
		Total:     200,
		PageSize:  48,
		HasServer: true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `id="media-list-sort"`)
	assert.Contains(t, body, `name="sort"`)
	assert.Contains(t, body, `hx-target="#media-browse"`)
	assert.Contains(t, body, `hx-include="closest form"`)
	assert.NotContains(t, body, `change from:select`)
	assert.Contains(t, body, `aria-label="Jump to letter"`)
	assert.Contains(t, body, `data-jump-key="H"`)
	assert.Contains(t, body, `id="media-prev"`)
	assert.Contains(t, body, "before=43")
	assert.Contains(t, body, `id="media-list-letter"`)
	assert.Contains(t, body, `name="letter"`)
	assert.Contains(t, body, "md:hidden")
	assert.Contains(t, body, "md:flex")
	assert.Contains(t, body, "overflow-y-auto")
	assert.Contains(t, body, "min-h-0")
	assert.Contains(t, body, "xl:grid-cols-6")
	assert.Contains(t, body, "pr-4")
	assert.NotContains(t, body, "sticky")
	assert.NotContains(t, body, "h-[calc(100dvh-4rem)]")
	assert.NotContains(t, body, "xl:grid-cols-4")
	assert.Contains(t, body, `letter=H`)
	assert.Contains(t, body, `href="/media/item/10"`)
	assert.Contains(t, body, `id="media-more"`)
	assert.Contains(t, body, "Loading more…")
	assert.Contains(t, body, "start=44")
	assert.Contains(t, body, "Loading previous…")
	assert.NotContains(t, body, ">Previous<")
	assert.NotContains(t, body, ">Next<")
	assert.Contains(t, body, "sort=title_asc")
}

func TestMediaBrowseShowsYearsForYearSort(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaBrowse(view.MediaProps{
		Items: []view.MediaItem{
			{ID: "10", Title: "Heat", Type: "movie", Year: 1995},
		},
		LibraryID: "1",
		Sort:      "year_desc",
		Letters: []view.LetterIndex{
			{Title: "1995", Size: 40, Start: 0},
		},
		Start:     0,
		Total:     200,
		PageSize:  48,
		HasServer: true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `id="media-list-sort"`)
	assert.Contains(t, body, `aria-label="Jump to year"`)
	assert.Contains(t, body, `data-jump-kind="year"`)
	assert.Contains(t, body, `id="media-list-letter"`)
	assert.Contains(t, body, `data-jump="1995"`)
	assert.Contains(t, body, `id="media-more"`)
	assert.NotContains(t, body, `id="media-prev"`)
}

func TestMediaBrowseOmitsLettersOnSearch(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaBrowse(view.MediaProps{
		Items: []view.MediaItem{
			{ID: "10", Title: "Heat", Type: "movie"},
		},
		LibraryID: "1",
		Query:     "heat",
		Sort:      "title_asc",
		Letters: []view.LetterIndex{
			{Title: "H", Size: 8, Start: 43},
		},
		Total:     1,
		PageSize:  48,
		HasServer: true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.NotContains(t, body, `id="media-list-sort"`)
	assert.NotContains(t, body, `aria-label="Jump to letter"`)
	assert.NotContains(t, body, `id="media-list-letter"`)
	assert.NotContains(t, body, `id="media-more"`)
}

func TestMediaBrowseOmitsLettersOnParent(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaBrowse(view.MediaProps{
		Items: []view.MediaItem{
			{ID: "11", Title: "Season 1", Type: "season"},
		},
		LibraryID: "1",
		ParentID:  "9",
		Sort:      "",
		Start:     0,
		Total:     1,
		PageSize:  48,
		HasServer: true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.NotContains(t, body, `id="media-list-sort"`)
	assert.NotContains(t, body, `aria-label="Jump to letter"`)
	assert.NotContains(t, body, `id="media-more"`)
}

func TestMediaBrowseParentInfiniteScroll(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaBrowse(view.MediaProps{
		Items: []view.MediaItem{
			{ID: "11", Title: "Episode 1", Type: "episode"},
		},
		LibraryID: "1",
		ParentID:  "9",
		Start:     0,
		Total:     80,
		PageSize:  48,
		HasServer: true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.NotContains(t, body, `id="media-list-sort"`)
	assert.NotContains(t, body, `aria-label="Jump to letter"`)
	assert.Contains(t, body, `id="media-more"`)
	assert.Contains(t, body, "parent=9")
}

func TestMediaResultsChooserLinksLibraries(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaResults(view.MediaProps{
		Libraries: []view.LibraryItem{
			{ID: "1", Title: "Movies", Type: "movie"},
			{ID: "2", Title: "TV Shows", Type: "show"},
		},
		HasServer: true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `href="/media?library=1"`)
	assert.Contains(t, body, `href="/media?library=2"`)
	assert.Contains(t, body, ">Movies<")
	assert.Contains(t, body, ">TV Shows<")
	assert.Contains(t, body, "flex-col")
	assert.NotContains(t, body, ">Browse<")
	assert.NotContains(t, body, "aspect-[2/3]")
}

func TestJumpKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "H", jumpKey(view.MediaItem{Title: "The Heat", TitleSort: "Heat"}, "title_asc"))
	assert.Equal(t, "#", jumpKey(view.MediaItem{Title: "2 Fast"}, "title_asc"))
	assert.Equal(t, "1995", jumpKey(view.MediaItem{Year: 1995}, "year_desc"))
	assert.Equal(t, "#", jumpKey(view.MediaItem{}, "year_asc"))
	assert.Equal(t, "03/2024", jumpKey(view.MediaItem{AddedAt: time.Date(2024, 3, 15, 0, 0, 0, 0, time.Local).Unix()}, "added_desc"))
}

func TestItemURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/media/item/10", string(itemURL(view.MediaItem{ID: "10", Title: "Heat"})))
	assert.Equal(t, "/media?library=1&parent=9", string(itemURL(view.MediaItem{
		ID:        "9",
		Browsable: true,
		BrowseURL: "/media?library=1&parent=9",
	})))
}

func TestMediaCardPosterLinksToItem(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := mediaCard(view.MediaItem{
		ID:        "10",
		Title:     "Heat",
		Type:      "movie",
		ThumbPath: "/thumbs?path=/library/metadata/10/thumb/1",
	}, "title_asc").Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.GreaterOrEqual(t, strings.Count(body, `href="/media/item/10"`), 2)
	assert.Contains(t, body, `alt="Heat"`)
	assert.Contains(t, body, ">Open<")
}

func TestMediaCardPosterLinksToBrowse(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := mediaCard(view.MediaItem{
		ID:        "9",
		Title:     "Strike Back",
		Type:      "show",
		Browsable: true,
		BrowseURL: "/media?library=2&parent=9",
	}, "title_asc").Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.GreaterOrEqual(t, strings.Count(body, `href="/media?library=2&amp;parent=9"`), 2)
	assert.Contains(t, body, ">Browse<")
	assert.NotContains(t, body, "/media/item/9")
}

func TestNextPageURLKeepsSortOmitsLetter(t *testing.T) {
	t.Parallel()

	got := nextPageURL(view.MediaProps{
		LibraryID: "1",
		Sort:      "title_asc",
		Letter:    "M",
		Items:     []view.MediaItem{{ID: "1"}, {ID: "2"}},
		Start:     43,
	})

	assert.Contains(t, got, "library=1")
	assert.Contains(t, got, "sort=title_asc")
	assert.Contains(t, got, "start=45")
	assert.NotContains(t, got, "letter=")
}

func TestMediaMoreOmitsCrumbs(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaMore(view.MediaProps{
		Items: []view.MediaItem{
			{ID: "10", Title: "Heat", Type: "movie"},
		},
		Crumbs: []view.Crumb{
			{Title: "Libraries", URL: "/media"},
			{Title: "Movies", URL: "/media?library=1"},
		},
		LibraryID: "1",
		Sort:      "title_asc",
		Start:     48,
		Total:     200,
		PageSize:  48,
		HasServer: true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "Heat")
	assert.Contains(t, body, `id="media-more"`)
	assert.NotContains(t, body, "Libraries")
	assert.NotContains(t, body, ">Movies<")
}
