// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package html

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestParseMediaListQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give string
		want mediaListQuery
	}{
		{
			give: "/media",
			want: mediaListQuery{},
		},
		{
			give: "/media?library=1",
			want: mediaListQuery{LibraryID: "1", Sort: mediaSortTitleAsc},
		},
		{
			give: "/media?library=1&sort=year_desc",
			want: mediaListQuery{LibraryID: "1", Sort: mediaSortYearDesc},
		},
		{
			give: "/media?library=1&sort=bogus",
			want: mediaListQuery{LibraryID: "1", Sort: mediaSortTitleAsc},
		},
		{
			give: "/media?library=1&letter=M",
			want: mediaListQuery{LibraryID: "1", Sort: mediaSortTitleAsc, Letter: "M"},
		},
		{
			give: "/media?library=1&sort=title_asc&letter=M&start=48",
			want: mediaListQuery{
				LibraryID: "1",
				Sort:      mediaSortTitleAsc,
				Letter:    "M",
				Start:     48,
			},
		},
		{
			give: "/media?library=1&parent=9&sort=title_asc&letter=A&start=48",
			want: mediaListQuery{LibraryID: "1", ParentID: "9", Start: 48},
		},
		{
			give: "/media?library=1&q=movie&sort=year_desc&letter=M",
			want: mediaListQuery{
				Query:     shared.DefaultMediaType,
				LibraryID: "1",
				Sort:      mediaSortYearDesc,
			},
		},
		{
			give: "/media?library=1&sort=title_asc&before=43",
			want: mediaListQuery{
				LibraryID: "1",
				Sort:      mediaSortTitleAsc,
				Before:    43,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.give, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, parseMediaListQueryFrom(t, test.give))
		})
	}
}

func TestMediaListQueryListStart(t *testing.T) {
	t.Parallel()

	index := []plex.LetterIndex{
		{Title: "#", Size: 3},
		{Title: "A", Size: 40},
		{Title: "M", Size: 8},
	}

	tests := []struct {
		name  string
		query mediaListQuery
		want  int
	}{
		{
			name:  "letter without start",
			query: mediaListQuery{Letter: "M"},
			want:  43,
		},
		{
			name:  "start wins over letter",
			query: mediaListQuery{Letter: "M", Start: 48},
			want:  48,
		},
		{
			name:  "zero start without letter",
			query: mediaListQuery{},
			want:  0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.query.listStart(index))
		})
	}
}

func TestMediaListQueryWindow(t *testing.T) {
	t.Parallel()

	index := []plex.LetterIndex{
		{Title: "#", Size: 3},
		{Title: "A", Size: 40},
	}

	window := (mediaListQuery{Before: 43}).window(index)
	assert.Equal(t, 0, window.Start)
	assert.Equal(t, 43, window.Size)

	window = (mediaListQuery{Before: 96}).window(index)
	assert.Equal(t, 48, window.Start)
	assert.Equal(t, 48, window.Size)

	window = (mediaListQuery{Letter: "A"}).window(index)
	assert.Equal(t, 3, window.Start)
	assert.Equal(t, shared.MediaPageSize, window.Size)
}

func TestAddedAtIndexes(t *testing.T) {
	t.Parallel()

	march := time.Date(2024, time.March, 2, 0, 0, 0, 0, time.Local).Unix()
	feb := time.Date(2024, time.February, 10, 0, 0, 0, 0, time.Local).Unix()

	got := addedAtIndexes([]plex.MediaItem{
		{AddedAt: march},
		{AddedAt: march},
		{AddedAt: feb},
	})

	assert.Equal(t, []plex.LetterIndex{
		{Title: "03/2024", Size: 2},
		{Title: "02/2024", Size: 1},
	}, got)
}

func TestTitleIndexes(t *testing.T) {
	t.Parallel()

	got := titleIndexes([]plex.MediaItem{
		{Title: "2 Movie", TitleSort: "2 Movie"},
		{Title: "The Movie", TitleSort: testMovie},
		{Title: "Movie 2", TitleSort: "Movie 2"},
		{Title: "Other Movie", TitleSort: "Other Movie"},
	})

	assert.Equal(t, []plex.LetterIndex{
		{Title: "#", Size: 1},
		{Title: "M", Size: 2},
		{Title: "O", Size: 1},
	}, got)
}

func TestYearIndexes(t *testing.T) {
	t.Parallel()

	got := yearIndexes([]plex.MediaItem{
		{Year: 2026},
		{Year: 2026},
		{Year: 2025},
		{Year: 0},
	})

	assert.Equal(t, []plex.LetterIndex{
		{Title: "2026", Size: 2},
		{Title: "2025", Size: 1},
		{Title: "#", Size: 1},
	}, got)
}

func TestThinJumpIndexesYearsUseDecades(t *testing.T) {
	t.Parallel()

	letters := make([]view.LetterIndex, 0, 93)
	start := 0
	for year := 2026; year >= 1934; year-- {
		letters = append(letters, view.LetterIndex{
			Title: strconv.Itoa(year),
			Size:  1,
			Start: start,
		})
		start++
	}

	got := thinJumpIndexes(letters, mediaSortYearDesc)
	assert.LessOrEqual(t, len(got), maxJumpLabels)
	assert.Equal(t, "2026", got[0].Title)
	assert.Equal(t, "1934", got[len(got)-1].Title)

	titles := make([]string, 0, len(got))
	for _, letter := range got {
		titles = append(titles, letter.Title)
	}

	assert.Contains(t, titles, "2020")
	assert.Contains(t, titles, "2010")
	assert.NotContains(t, titles, "2023")
}

func TestThinJumpIndexesKeepsShortLists(t *testing.T) {
	t.Parallel()

	letters := []view.LetterIndex{
		{Title: "2026", Size: 4, Start: 0},
		{Title: "2025", Size: 2, Start: 4},
		{Title: "2024", Size: 1, Start: 6},
	}

	assert.Equal(t, letters, thinJumpIndexes(letters, mediaSortYearDesc))
}

func TestThinMonthTitlesKeepsOnePerYear(t *testing.T) {
	t.Parallel()

	letters := make([]view.LetterIndex, 0, 40)

	for i := range 40 {
		year := 2026 - i/12
		month := 12 - i%12

		letters = append(letters, view.LetterIndex{
			Title: time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Format("01/2006"),
			Size:  1,
			Start: i,
		})
	}

	got := thinMonthTitles(letters)
	assert.LessOrEqual(t, len(got), maxJumpLabels)
	assert.Greater(t, len(got), 2)
}

func TestOrderJumpIndex(t *testing.T) {
	t.Parallel()

	letters := []plex.LetterIndex{{Title: "A", Size: 2}, {Title: "B", Size: 3}}
	assert.Equal(
		t,
		[]plex.LetterIndex{{Title: "B", Size: 3}, {Title: "A", Size: 2}},
		orderJumpIndex(letters, mediaSortTitleDesc),
	)

	years := []plex.LetterIndex{{Title: "1995", Size: 2}, {Title: "2024", Size: 1}}
	assert.Equal(
		t,
		[]plex.LetterIndex{{Title: "2024", Size: 1}, {Title: "1995", Size: 2}},
		orderJumpIndex(years, mediaSortYearDesc),
	)
}

func TestPlexMediaSort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give string
		want string
	}{
		{give: mediaSortTitleAsc, want: "titleSort:asc"},
		{give: mediaSortTitleDesc, want: "titleSort:desc"},
		{give: mediaSortYearDesc, want: "year:desc"},
		{give: mediaSortYearAsc, want: "year:asc"},
		{give: mediaSortAddedDesc, want: "addedAt:desc"},
		{give: mediaSortAddedAsc, want: "addedAt:asc"},
		{give: "", want: ""},
		{give: "bogus", want: ""},
	}

	for _, test := range tests {
		t.Run(test.give, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, plexMediaSort(test.give))
		})
	}
}

func TestToLetterIndexesOmitsEmpty(t *testing.T) {
	t.Parallel()

	got := toLetterIndexes([]plex.LetterIndex{
		{Title: "#", Size: 3},
		{Title: "A", Size: 0},
		{Title: "B", Size: 12},
	})

	assert.Equal(t, []view.LetterIndex{
		{Title: "#", Size: 3, Start: 0},
		{Title: "B", Size: 12, Start: 3},
	}, got)
}

func parseMediaListQueryFrom(t *testing.T, target string) mediaListQuery {
	t.Helper()

	app := fiber.New()
	var parsed mediaListQuery

	app.Get("/media", func(ctx fiber.Ctx) error {
		parsed = parseMediaListQuery(ctx)

		return nil
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	_, _ = io.Copy(io.Discard, resp.Body)

	return parsed
}

func TestCollectAddedAtItemsStopsAtPageCap(t *testing.T) {
	t.Parallel()

	calls := 0
	client, server := addedAtPlexClient(t, func(http.ResponseWriter, *http.Request) {
		calls++
	})

	items := collectAddedAtItems(t.Context(), client, server, "1", mediaSortAddedDesc)

	assert.Equal(t, maxAddedIndexPages, calls)
	assert.Len(t, items, maxAddedIndexPages)
}

func TestLoadAddedAtIndexesCaches(t *testing.T) {
	t.Parallel()

	calls := 0
	client, server := addedAtPlexClient(t, func(http.ResponseWriter, *http.Request) {
		calls++
	})
	cache := &addedAtIndexCache{}

	first := loadAddedAtIndexes(t.Context(), cache, client, server, "1", mediaSortAddedDesc)
	second := loadAddedAtIndexes(t.Context(), cache, client, server, "1", mediaSortAddedDesc)

	assert.Equal(t, first, second)
	assert.Equal(t, maxAddedIndexPages, calls)
}

func addedAtPlexClient(
	t *testing.T,
	onRequest func(http.ResponseWriter, *http.Request),
) (*plex.Client, plex.Server) {
	t.Helper()

	handler := func(writer http.ResponseWriter, request *http.Request) {
		onRequest(writer, request)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)

		_, _ = writer.Write([]byte(
			`{"MediaContainer":{"size":1,"totalSize":5000,"offset":0,"Metadata":[` +
				`{"ratingKey":"100","title":"Paged","type":"movie","addedAt":1700000000}` +
				`]}}`,
		))
	}
	ts := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(ts.Close)

	parsed, err := url.Parse(ts.URL)
	require.NoError(t, err)

	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)

	client := plex.NewClient(plex.ClientConfig{
		Product:  "outtake",
		ClientID: "test",
		Token:    "token",
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})
	server := plex.Server{
		Address: parsed.Hostname(),
		Port:    port,
		Token:   "token",
		Scheme:  "http",
	}

	return client, server
}
