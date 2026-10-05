// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
)

func TestAddedAtIndexes(t *testing.T) {
	t.Parallel()

	march := time.Date(2024, time.March, 2, 0, 0, 0, 0, time.Local).Unix()
	feb := time.Date(2024, time.February, 10, 0, 0, 0, 0, time.Local).Unix()

	got := AddedAtIndexes([]plex.MediaItem{
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

	got := TitleIndexes([]plex.MediaItem{
		{Title: "2 Movie", TitleSort: "2 Movie"},
		{Title: "The Movie", TitleSort: "Movie"},
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

	got := YearIndexes([]plex.MediaItem{
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

func TestJumpMarksThinsCrowdedYears(t *testing.T) {
	t.Parallel()

	buckets := make([]plex.LetterIndex, 0, 93)
	for year := 2026; year >= 1934; year-- {
		buckets = append(buckets, plex.LetterIndex{
			Title: strconv.Itoa(year),
			Size:  1,
		})
	}

	got := JumpMarks(buckets, SortYearDesc)
	assert.LessOrEqual(t, len(got), maxJumpLabels)
	assert.Equal(t, "2026", got[0].Title)
	assert.Equal(t, "1934", got[len(got)-1].Title)

	titles := make([]string, 0, len(got))
	for _, mark := range got {
		titles = append(titles, mark.Title)
	}

	assert.Contains(t, titles, "2020")
	assert.Contains(t, titles, "2010")
	assert.NotContains(t, titles, "2023")
}

func TestJumpMarksKeepsShortLists(t *testing.T) {
	t.Parallel()

	got := JumpMarks([]plex.LetterIndex{
		{Title: "2026", Size: 4},
		{Title: "2025", Size: 2},
		{Title: "2024", Size: 1},
	}, SortYearDesc)

	assert.Len(t, got, 3)
	assert.Equal(t, "2026", got[0].Title)
	assert.Equal(t, 4, got[0].Size)
	assert.Equal(t, "2025", got[1].Title)
	assert.Equal(t, 2, got[1].Size)
	assert.Equal(t, "2024", got[2].Title)
	assert.Equal(t, 1, got[2].Size)
}

func TestJumpMarksOmitsEmptyBuckets(t *testing.T) {
	t.Parallel()

	got := JumpMarks([]plex.LetterIndex{
		{Title: "#", Size: 3},
		{Title: "A", Size: 0},
		{Title: "B", Size: 12},
	}, SortTitleAsc)

	assert.Equal(t, []LetterIndex{
		{Title: "#", Size: 3},
		{Title: "B", Size: 12},
	}, got)
}

func TestThinMonthMarksKeepsOnePerYear(t *testing.T) {
	t.Parallel()

	marks := make([]LetterIndex, 0, 40)

	for i := range 40 {
		year := 2026 - i/12
		month := 12 - i%12

		marks = append(marks, LetterIndex{
			Title: time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Format("01/2006"),
			Size:  1,
		})
	}

	got := thinMonthMarks(marks)
	assert.LessOrEqual(t, len(got), maxJumpLabels)
	assert.Greater(t, len(got), 2)
}

func TestOrderIndex(t *testing.T) {
	t.Parallel()

	letters := []plex.LetterIndex{{Title: "A", Size: 2}, {Title: "B", Size: 3}}
	assert.Equal(
		t,
		[]plex.LetterIndex{{Title: "B", Size: 3}, {Title: "A", Size: 2}},
		OrderIndex(letters, SortTitleDesc),
	)

	years := []plex.LetterIndex{{Title: "1995", Size: 2}, {Title: "2024", Size: 1}}
	assert.Equal(
		t,
		[]plex.LetterIndex{{Title: "2024", Size: 1}, {Title: "1995", Size: 2}},
		OrderIndex(years, SortYearDesc),
	)

	assert.Equal(t, years, OrderIndex(years, SortTitleAsc))
}

func TestCollectAddedAtItemsStopsAtPageCap(t *testing.T) {
	t.Parallel()

	calls := 0
	client, server := addedAtPlexClient(t, func(http.ResponseWriter, *http.Request) {
		calls++
	})

	items := collectAddedAtItems(t.Context(), client, server, "1", SortAddedDesc)

	assert.Equal(t, maxAddedIndexPages, calls)
	assert.Len(t, items, maxAddedIndexPages)
}

func TestAddedAtCacheLoadCaches(t *testing.T) {
	t.Parallel()

	calls := 0
	client, server := addedAtPlexClient(t, func(http.ResponseWriter, *http.Request) {
		calls++
	})
	cache := &AddedAtCache{}

	first := cache.Load(t.Context(), client, server, "1", SortAddedDesc)
	second := cache.Load(t.Context(), client, server, "1", SortAddedDesc)

	assert.Equal(t, first, second)
	assert.Equal(t, maxAddedIndexPages, calls)
}

func TestAddedAtCacheLoadCopiesEntries(t *testing.T) {
	t.Parallel()

	client, server := addedAtPlexClient(t, func(http.ResponseWriter, *http.Request) {})
	cache := &AddedAtCache{}

	first := cache.Load(t.Context(), client, server, "1", SortAddedDesc)
	size := first[0].Size

	first[0].Size = size + 1

	assert.Equal(t, size, cache.Load(t.Context(), client, server, "1", SortAddedDesc)[0].Size)
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
