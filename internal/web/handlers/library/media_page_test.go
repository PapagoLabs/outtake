// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// mediaApp mounts the media library routes on a fresh app.
//
// Parameters:
//   - handler: The handler under test.
//
// Returns:
//   - app: The app the routes are mounted on.
func mediaApp(handler *Handler) *fiber.App {
	app := fiber.New()
	app.Get(routes.PathMedia, handler.Media)

	return app
}

// getMedia serves one media library page request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - target: Request target, including any query string.
//   - hxTarget: HX-Target header value, empty to omit the header.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func getMedia(
	t *testing.T,
	handler *Handler,
	target string,
	hxTarget string,
) pageAnswer {
	t.Helper()

	app := mediaApp(handler)

	if hxTarget != "" {
		app.Use(func(ctx fiber.Ctx) error {
			ctx.Request().Header.Set(routes.HeaderHXTarget, hxTarget)

			return ctx.Next()
		})
	}

	return serveMethod(t, app, http.MethodGet, target, hxTarget != "", hxTarget, "")
}

// browseStub starts a Plex Media Server serving one library listing and one
// title listing.
//
// Parameters:
//   - t: The test the server belongs to.
//
// Returns:
//   - stub: The stub, closed when the test ends.
func browseStub(t *testing.T) *pmsStub {
	t.Helper()

	return startPMS(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/sections/all" {
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"},
		{"key":"2","title":"TV Shows","type":"show"}
	]}}`))

			return
		}

		_, _ = w.Write([]byte(`{"MediaContainer":{"size":2,
		"Metadata":[
			{"ratingKey":"42","title":"Alpha","type":"movie","year":1999,
			 "addedAt":1000,"thumb":"/library/metadata/42/thumb/1",
			 "Media":[{"id":"7","tag":"7"}]},
			{"ratingKey":"43","title":"Beta","type":"movie","year":2001,
			 "addedAt":2000,"thumb":"/library/metadata/43/thumb/1"}
		],
		"Directory":[{"key":"1","title":"Movies","type":"movie"}]}}`))
	})
}

func TestMediaRendersTheLibraryChooserWithNoServerBound(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := getMedia(t, handler, routes.PathMedia, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Media Libraries",
		"with nothing bound the page still explains what it is")
}

func TestMediaRendersTheLibraryChooser(t *testing.T) {
	t.Parallel()

	stub := browseStub(t)

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getMedia(t, handler, routes.PathMedia, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Movies",
		"a library the server reported has to be offered as a card")
	assertBodyContains(t, answer.body, "TV Shows",
		"every section is offered, not just the first")
}

func TestMediaRendersALibraryListing(t *testing.T) {
	t.Parallel()

	stub := browseStub(t)

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getMedia(t, handler,
		routes.PathMedia+"?"+url.Values{routes.QueryLibrary: {"1"}}.Encode(), "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Alpha",
		"a title in the chosen library has to be on the page")
	assertBodyOmits(t, answer.body, "TV Shows",
		"once a library is chosen the chooser is not the page")
}

func TestMediaRendersTheResultsPaneForHTMX(t *testing.T) {
	t.Parallel()

	stub := browseStub(t)

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getMedia(t, handler, sectionMediaTarget(), routes.TargetMediaBrowse)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Alpha",
		"the pane the browse swaps in carries the titles on its own")
	assertBodyOmits(t, answer.body, "<html",
		"a targeted swap answers with the pane, not a whole document")
}

func TestMediaRendersTheNextPosterPage(t *testing.T) {
	t.Parallel()

	stub := browseStub(t)

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getMedia(t, handler, sectionMediaTarget(), routes.TargetMediaMore)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Alpha",
		"appending a page still has to carry the titles it adds")
	assertBodyOmits(t, answer.body, "<html",
		"a targeted swap answers with the fragment, not a whole document")
}

func TestMediaRendersThePreviousPosterPage(t *testing.T) {
	t.Parallel()

	stub := browseStub(t)

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getMedia(t, handler, pagedMediaTarget(), routes.TargetMediaPrev)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Alpha",
		"prepending a page still has to carry the titles it adds")
	assertBodyOmits(t, answer.body, "<html",
		"a targeted swap answers with the fragment, not a whole document")
}

func TestMediaRendersAnEmptyPageWhenPlexIsUnreachable(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getMedia(t, handler, routes.PathMedia, "")

	require.Equal(t, fiber.StatusOK, answer.status,
		"a browse that failed leaves the page empty rather than erroring")
	assertBodyOmits(t, answer.body, "Alpha",
		"nothing was read, so no title can be claimed")
}

func TestMediaFollowsTheCarriedQuery(t *testing.T) {
	t.Parallel()

	stub := browseStub(t)

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	target := routes.PathMedia + "?" + url.Values{
		routes.QueryLibrary: {"1"},
		queryQ:              {"Beta"},
		querySort:           {library.SortTitleAsc},
	}.Encode()

	answer := getMedia(t, handler, target, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `value="Beta"`,
		"the search the page was opened with is prefilled")
	assertBodyContains(t, answer.body, "Beta",
		"the search reaches the server, so the hits come back")
}

func TestMediaCarriesTheJumpRailLetter(t *testing.T) {
	t.Parallel()

	stub := browseStub(t)

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	target := routes.PathMedia + "?" + url.Values{
		routes.QueryLibrary: {"1"},
		queryLetter:         {"A"},
	}.Encode()

	answer := getMedia(t, handler, target, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `data-jump="A"`,
		"each card is tagged with its jump bucket so the chosen letter can be found")
	assertBodyContains(t, answer.body, `data-jump="B"`,
		"every letter on the rail has a card it can scroll to")
}

func TestMediaSkipsTheJumpRailForAPagedRequest(t *testing.T) {
	t.Parallel()

	stub := browseStub(t)

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getMedia(t, handler, pagedMediaTarget(), routes.TargetMediaMore)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyOmits(t, answer.body, `hx-get="/media" hx-target="#media-letters"`,
		"appending a page must not rebuild a jump rail that is already on the page")
}

func TestParseMediaListQueryReadsEveryBrowseParameter(t *testing.T) {
	t.Parallel()

	target := routes.PathMedia + "?" + url.Values{
		routes.QueryLibrary: {"1"},
		querySort:           {library.SortTitleDesc},
		queryLetter:         {"B"},
		routes.QueryStart:   {"200"},
		queryBefore:         {"100"},
	}.Encode()

	query := queryOf(t, target)

	assert.Equal(t, "1", query.LibraryID)
	assert.Equal(t, library.SortTitleDesc, query.Sort)
	assert.Equal(t, 200, query.Start)
	assert.Equal(t, 100, query.Before)
	assert.Equal(t, "B", query.Letter,
		"a jump-rail letter only survives when nothing else narrows the listing")
}

func TestParseMediaListQueryDropsALetterForASearch(t *testing.T) {
	t.Parallel()

	target := routes.PathMedia + "?" + url.Values{
		routes.QueryLibrary: {"1"},
		queryQ:              {"needle"},
		queryLetter:         {"B"},
		queryBefore:         {"100"},
	}.Encode()

	query := queryOf(t, target)

	assert.Empty(t, query.Letter,
		"a letter bucket and a search cannot both apply to one listing")
	assert.Zero(t, query.Before,
		"a search has one page, so there is nothing to prepend")
}

func TestParseMediaListQueryOrdersAChildFolderByHand(t *testing.T) {
	t.Parallel()

	target := routes.PathMedia + "?" + url.Values{
		routes.QueryLibrary: {"1"},
		routes.QueryParent:  {"7"},
		querySort:           {library.SortTitleDesc},
		queryLetter:         {"B"},
		queryBefore:         {"100"},
	}.Encode()

	query := queryOf(t, target)

	assert.Equal(t, "7", query.ParentID)
	assert.Empty(t, query.Sort,
		"a folder listing is ordered by the folder, so the section order is dropped")
	assert.Empty(t, query.Letter,
		"a folder listing has no jump rail")
	assert.Zero(t, query.Before,
		"a folder listing is not paged")
}

func TestParseMediaListQueryDefaultsASectionToTitles(t *testing.T) {
	t.Parallel()

	target := routes.PathMedia + "?" + url.Values{routes.QueryLibrary: {"1"}}.Encode()

	assert.Equal(t, library.SortTitleAsc, queryOf(t, target).Sort,
		"a section listing with no ordering falls back to titles A-Z")
}

func TestParseMediaListQueryCarriesNoOrderingForTheRoot(t *testing.T) {
	t.Parallel()

	query := queryOf(t, routes.PathMedia)

	assert.Empty(t, query.Sort,
		"an empty form carries no ordering, so the chooser renders its own default")
	assert.Equal(t, 0, query.Start)
	assert.Equal(t, 0, query.Before)
}

func TestParseMediaListQueryRejectsAnUnreadableStart(t *testing.T) {
	t.Parallel()

	target := routes.PathMedia + "?" + url.Values{
		routes.QueryStart: {"later"},
		queryBefore:       {"earlier"},
	}.Encode()

	query := queryOf(t, target)

	assert.Zero(t, query.Start, "an offset the server cannot read carries none")
	assert.Zero(t, query.Before)
}

// pagedMediaTarget is a media page request with a library chosen and a listing
// already part way through, so there is a page on both sides of it.
//
// Returns:
//   - target: The request target.
func pagedMediaTarget() string {
	return routes.PathMedia + "?" + url.Values{
		routes.QueryLibrary: {"1"},
		routes.QueryStart:   {"50"},
	}.Encode()
}

// queryOf reads the browse state a media page request carries.
//
// Parameters:
//   - t: The test the read belongs to.
//   - target: Request target, including any query string.
//
// Returns:
//   - query: The normalized browse state.
func queryOf(t *testing.T, target string) library.Query {
	t.Helper()

	var query library.Query

	app := fiber.New()
	app.Get(routes.PathMedia, func(ctx fiber.Ctx) error {
		query = parseMediaListQuery(ctx)

		return ctx.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, target, nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	return query
}

// sectionMediaTarget is a media page request scoped to one library section.
//
// Returns:
//   - target: The request target.
func sectionMediaTarget() string {
	return routes.PathMedia + "?" + url.Values{routes.QueryLibrary: {"1"}}.Encode()
}
