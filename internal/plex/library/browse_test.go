// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// browsePMS serves canned responses per Plex path and records what was asked.
//
// Parameters:
//   - t: The test the client belongs to.
//   - bodies: Response body per Plex path.
//
// Returns:
//   - client: PMS client pointed at the stub.
//   - server: PMS the client queries.
func browsePMS(t *testing.T, bodies map[string]string) (*plex.Client, plex.Server) {
	t.Helper()

	ts := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			body, ok := bodies[request.URL.Path]
			if !ok {
				body = `{"MediaContainer":{"size":0}}`
			}

			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)

			_, _ = writer.Write([]byte(body))
		}),
	)
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
	})

	return client, plex.Server{
		Address: parsed.Hostname(),
		Port:    port,
		Token:   "token",
		Scheme:  "http",
	}
}

// sectionPath is the library section endpoint for one library.
func sectionPath(libraryID string) string {
	return "/library/sections/" + libraryID + "/all"
}

func TestBrowseListsTheLibraryChooser(t *testing.T) {
	t.Parallel()

	// librariesJSON is one library section, as the sections endpoint reports it.
	const librariesJSON = `{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"}
	]}}`

	client, server := browsePMS(t, map[string]string{
		"/library/sections/all": librariesJSON,
	})

	browsed, err := Browse(
		t.Context(),
		&LibraryCache{},
		client,
		server,
		Query{},
		Window{Size: PageSize},
	)
	require.NoError(t, err)

	require.Len(t, browsed.Libraries, 1)
	assert.Equal(t, "1", browsed.Libraries[0].ID)
	assert.Empty(t, browsed.Items, "the chooser shows libraries, not media")
	assert.Zero(t, browsed.Total)
}

func TestBrowseListsALibrarySectionWindow(t *testing.T) {
	t.Parallel()

	// librariesJSON is one library section, as the sections endpoint reports it.
	const librariesJSON = `{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"}
	]}}`

	// sectionJSON is one page of a library section.
	const sectionJSON = `{"MediaContainer":{"size":1,"totalSize":120,"offset":48,"Metadata":[
		{"ratingKey":"501","title":"Second","type":"movie"}
	]}}`

	// firstCharacterJSON is one letter bucket.
	const firstCharacterJSON = `{"MediaContainer":{"Directory":[
		{"key":"S","title":"S","size":4}
	]}}`

	client, server := browsePMS(t, map[string]string{
		sectionPath("1"):                     sectionJSON,
		"/library/sections/all":              librariesJSON,
		"/library/sections/1/firstCharacter": firstCharacterJSON,
	})

	query := NormalizeQuery(Query{LibraryID: "1"})
	window := query.Window(nil)

	browsed, err := Browse(t.Context(), &LibraryCache{}, client, server, query, window)
	require.NoError(t, err)

	require.Len(t, browsed.Items, 1)
	assert.Equal(t, "Second", browsed.Items[0].Title)
	assert.Equal(t, 120, browsed.Total)
	assert.Len(t, browsed.Libraries, 1, "the crumbs still need the libraries")
}

func TestBrowseListsContainerChildren(t *testing.T) {
	t.Parallel()

	// librariesJSON is one library section, as the sections endpoint reports it.
	const librariesJSON = `{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"}
	]}}`

	// childrenJSON is one page of container children.
	const childrenJSON = `{"MediaContainer":{"size":1,"totalSize":3,"offset":0,"Metadata":[
		{"ratingKey":"601","title":"Episode","type":"episode"}
	]}}`

	client, server := browsePMS(t, map[string]string{
		"/library/metadata/9/children": childrenJSON,
		"/library/sections/all":        librariesJSON,
	})

	query := NormalizeQuery(Query{LibraryID: "1", ParentID: "9"})
	window := query.Window(nil)

	browsed, err := Browse(t.Context(), &LibraryCache{}, client, server, query, window)
	require.NoError(t, err)

	require.Len(t, browsed.Items, 1)
	assert.Equal(t, "Episode", browsed.Items[0].Title)
	assert.Equal(t, 3, browsed.Total)
}

func TestBrowseSearchesInsteadOfListing(t *testing.T) {
	t.Parallel()

	// librariesJSON is one library section, as the sections endpoint reports it.
	const librariesJSON = `{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"}
	]}}`

	// searchJSON is one search hit.
	const searchJSON = `{"MediaContainer":{"Hub":[
		{"title":"Movies","type":"movie","Metadata":[
			{"ratingKey":"701","title":"Hit","type":"movie"}
		]}
	]}}`

	client, server := browsePMS(t, map[string]string{
		"/hubs/search":          searchJSON,
		"/library/sections/all": librariesJSON,
	})

	query := NormalizeQuery(Query{Query: "hit", LibraryID: "1"})

	browsed, err := Browse(t.Context(), &LibraryCache{}, client, server, query, query.Window(nil))
	require.NoError(t, err)

	require.Len(t, browsed.Items, 1)
	assert.Equal(t, "Hit", browsed.Items[0].Title)
	assert.Equal(t, 1, browsed.Total)
}

func TestBrowseReportsALibraryLookupFailure(t *testing.T) {
	t.Parallel()

	client, server := browsePMS(t, map[string]string{
		"/library/sections/all": "not json",
	})

	_, err := Browse(t.Context(), &LibraryCache{}, client, server, Query{}, Window{Size: PageSize})
	require.ErrorContains(t, err, "list libraries")
}

func TestBrowseKeepsLibrariesWhenTheWindowFails(t *testing.T) {
	t.Parallel()

	// librariesJSON is one library section, as the sections endpoint reports it.
	const librariesJSON = `{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"}
	]}}`

	client, server := browsePMS(t, map[string]string{
		"/library/sections/all": librariesJSON,
		sectionPath("1"):        "not json",
	})

	query := NormalizeQuery(Query{LibraryID: "1"})

	browsed, err := Browse(t.Context(), &LibraryCache{}, client, server, query, query.Window(nil))
	require.ErrorContains(t, err, "list section")
	assert.Len(t, browsed.Libraries, 1, "the sidebar survives a failed listing")
	assert.Empty(t, browsed.Items)
}

func TestJumpIndexIsAbsentOffALibraryRoot(t *testing.T) {
	t.Parallel()

	client, server := browsePMS(t, nil)

	for _, query := range []Query{
		{},
		NormalizeQuery(Query{Query: "hit", LibraryID: "1"}),
		NormalizeQuery(Query{LibraryID: "1", ParentID: "9"}),
	} {
		index, err := JumpIndex(t.Context(), client, server, query, nil)
		require.NoError(t, err)
		assert.Empty(t, index)
	}
}

func TestJumpIndexCollectsLettersBySort(t *testing.T) {
	t.Parallel()

	// firstCharacterJSON is one letter bucket.
	const firstCharacterJSON = `{"MediaContainer":{"Directory":[
		{"key":"S","title":"S","size":4}
	]}}`

	client, server := browsePMS(t, map[string]string{
		"/library/sections/1/firstCharacter": firstCharacterJSON,
	})

	query := NormalizeQuery(Query{LibraryID: "1", Sort: SortTitleAsc})

	index, err := JumpIndex(t.Context(), client, server, query, nil)
	require.NoError(t, err)

	require.Len(t, index, 1)
	assert.Equal(t, "S", index[0].Title)
	assert.Equal(t, 4, index[0].Size)
}

func TestJumpIndexCollectsYearsForYearSorts(t *testing.T) {
	t.Parallel()

	client, server := browsePMS(t, map[string]string{
		"/library/sections/1/year": `{"MediaContainer":{"Directory":[
			{"key":"1999","title":"1999","size":2}
		]}}`,
	})

	query := NormalizeQuery(Query{LibraryID: "1", Sort: SortYearDesc})

	index, err := JumpIndex(t.Context(), client, server, query, nil)
	require.NoError(t, err)

	require.Len(t, index, 1)
	assert.Equal(t, "1999", index[0].Title)
}

func TestJumpIndexNeedsACacheForAddedAtSorts(t *testing.T) {
	t.Parallel()

	client, server := browsePMS(t, nil)

	query := NormalizeQuery(Query{LibraryID: "1", Sort: SortAddedDesc})

	_, err := JumpIndex(t.Context(), client, server, query, nil)
	require.ErrorIs(t, err, errNoAddedAtCache)

	index, cacheErr := JumpIndex(t.Context(), client, server, query, &AddedAtCache{})
	require.NoError(t, cacheErr)
	assert.Empty(t, index)
}

func TestJumpIndexReportsAFacetFailure(t *testing.T) {
	t.Parallel()

	client, server := browsePMS(t, map[string]string{
		"/library/sections/1/firstCharacter": "not json",
	})

	query := NormalizeQuery(Query{LibraryID: "1", Sort: SortTitleAsc})

	_, err := JumpIndex(t.Context(), client, server, query, nil)
	require.ErrorContains(t, err, "list firstCharacter")
}

func TestBrowseReportsASearchFailure(t *testing.T) {
	t.Parallel()

	// librariesJSON is one library section, as the sections endpoint reports it.
	const librariesJSON = `{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"}
	]}}`

	client, server := browsePMS(t, map[string]string{
		"/hubs/search":          "not json",
		"/library/sections/all": librariesJSON,
	})

	query := NormalizeQuery(Query{Query: "hit", LibraryID: "1"})

	_, err := Browse(t.Context(), &LibraryCache{}, client, server, query, query.Window(nil))
	require.ErrorContains(t, err, "search media")
}

func TestBrowseAsksForTheWindowItWasGiven(t *testing.T) {
	t.Parallel()

	var asked string

	ts := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if strings.Contains(request.URL.Path, "/all") {
				asked = request.URL.RawQuery
			}

			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)

			_, _ = writer.Write([]byte(`{"MediaContainer":{"size":0,"totalSize":0}}`))
		}),
	)
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
	})
	server := plex.Server{
		Address: parsed.Hostname(),
		Port:    port,
		Token:   "token",
		Scheme:  "http",
	}

	query := NormalizeQuery(Query{LibraryID: "1"})

	_, err = Browse(
		t.Context(),
		&LibraryCache{},
		client,
		server,
		query,
		Window{Start: 96, Size: PageSize},
	)
	require.NoError(t, err)

	assert.Contains(t, asked, "X-Plex-Container-Start=96")
	assert.Contains(t, asked, "X-Plex-Container-Size=48")
}
