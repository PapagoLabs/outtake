// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/library/media/mocks"
)

type served struct {
	status int
	body   string
}

// served is what one served request produced.

// mediaTestServer starts a loopback server standing in for a PMS.
//
// Parameters:
//   - t: The test the server belongs to.
//   - handler: Handler answering every request.
//
// Returns:
//   - ts: The loopback server, closed when the test ends.
func mediaTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return ts
}

// writeJSON answers one PMS request with a JSON body.
//
// Parameters:
//   - w: Response writer to answer on.
//   - body: JSON body to send.
func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	_, _ = w.Write([]byte(body))
}

// pmsFor points a Plex client at a loopback server.
//
// Parameters:
//   - t: The test the server belongs to.
//   - ts: The loopback server standing in for a PMS.
//
// Returns:
//   - client: A Plex client with no token of its own.
//   - server: A Plex server whose origin is ts.
func pmsFor(t *testing.T, ts *httptest.Server) (*plex.Client, plex.Server) {
	t.Helper()

	host, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	require.NoError(t, err)

	portNumber, err := strconv.Atoi(port)
	require.NoError(t, err)

	client := plex.NewClient(plex.ClientConfig{
		Product: "outtake-test",
		Timeout: 5 * time.Second,
	})

	return client, plex.Server{
		Name:    "loopback",
		Address: host,
		Port:    portNumber,
		Scheme:  "http",
	}
}

// serve runs one request against a handler mounted on a path.
//
// Parameters:
//   - t: The test the request belongs to.
//   - method: HTTP method to issue.
//   - path: Route to mount and request.
//   - target: Request target, including any query string.
//   - handler: The handler under test.
//
// Returns:
//   - served: The status and body the handler wrote.
func serve(
	t *testing.T,
	method, path, target string,
	handler fiber.Handler,
) served {
	t.Helper()

	app := fiber.New()
	app.Add([]string{method}, path, handler)

	req := httptest.NewRequestWithContext(t.Context(), method, target, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return served{status: resp.StatusCode, body: string(body)}
}

// getSessions serves one live-sessions request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//
// Returns:
//   - served: The status and body the handler wrote.
func getSessions(t *testing.T, handler *Handler) served {
	t.Helper()

	return serve(t, http.MethodGet, "/api/plex/sessions", "/api/plex/sessions", handler.GetSessions)
}

// search serves one media-search request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - target: Request target, including any query string.
//
// Returns:
//   - served: The status and body the handler wrote.
func search(t *testing.T, handler *Handler, target string) served {
	t.Helper()

	return serve(t, http.MethodGet, "/api/plex/search", target, handler.Search)
}

func TestNewKeepsTheLookupItWasGiven(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexLookup(t)

	assert.Equal(t, &Handler{auth: auth}, New(auth))
}

func TestGetSessionsServesTheCachedSessions(t *testing.T) {
	t.Parallel()

	sessions := []plex.Session{{
		ID:         "session-1",
		MediaItem:  plex.MediaItem{ID: "100", Title: "Test Movie", Type: "movie"},
		Title:      "Test Movie",
		ViewOffset: 900,
	}}

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Sessions().Return(sessions)

	got := getSessions(t, New(auth))

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.JSONEq(t,
		`[{"id":"session-1","mediaId":"100","title":"Test Movie","duration":0,"viewOffset":900}]`,
		got.body,
		"the cached sessions answer without troubling the PMS")
}

func TestGetSessionsServesAnEmptyArrayWhenNothingHasPolled(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Sessions().Return(nil)
	auth.EXPECT().Client().Return(nil, plex.EmptyServer(), false)

	got := getSessions(t, New(auth))

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.JSONEq(t, `[]`, got.body, "no server bound means nothing is playing")
}

func TestGetSessionsPollsTheServerWhenNothingHasCached(t *testing.T) {
	t.Parallel()

	ts := mediaTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/status/sessions", r.URL.Path)
		writeJSON(w, `{"MediaContainer":{"Metadata":[
			{"ratingKey":"100","title":"Test Movie","type":"movie","duration":7200000,
			 "viewOffset":900000,"Session":{"id":"session-1"}}
		]}}`)
	})

	client, server := pmsFor(t, ts)

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Sessions().Return(nil)
	auth.EXPECT().Client().Return(client, server, true)

	got := getSessions(t, New(auth))

	require.Equal(t, fiber.StatusOK, got.status)
	assert.JSONEq(
		t,
		`[{"id":"session-1","mediaId":"100","title":"Test Movie","duration":7200,"viewOffset":900}]`,
		got.body,
		"a live poll stands in for the missing cache",
	)
}

func TestGetSessionsReportsAnEmptyArrayWhenThePollFails(t *testing.T) {
	t.Parallel()

	ts := mediaTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	client, server := pmsFor(t, ts)

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Sessions().Return(nil)
	auth.EXPECT().Client().Return(client, server, true)

	got := getSessions(t, New(auth))

	assert.Equal(t, fiber.StatusOK, got.status,
		"a page that polls cannot be handed an error status")
	assert.JSONEq(t, `[]`, got.body, "an unreachable PMS is reported as nothing playing")
}

func TestSearchRequiresAQuery(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexLookup(t)

	got := search(t, New(auth), "/api/plex/search")

	assert.Equal(t, fiber.StatusBadRequest, got.status)
	assert.JSONEq(t,
		`{"error":"missing_query","message":"Enter something to search for"}`, got.body)
}

func TestSearchReturnsAnEmptyListWithNoServerBound(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Client().Return(nil, plex.EmptyServer(), false)

	got := search(t, New(auth), "/api/plex/search"+"?q=intro")

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.JSONEq(t, `{"items":[],"total":0}`, got.body,
		"a search with nothing to search reports no results, not a failure")
}

func TestSearchReportsAFailedSearch(t *testing.T) {
	t.Parallel()

	ts := mediaTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client, server := pmsFor(t, ts)

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Client().Return(client, server, true)

	got := search(t, New(auth), "/api/plex/search"+"?q=intro")

	assert.Equal(t, fiber.StatusInternalServerError, got.status)
	assert.Contains(t, got.body, "search_failed",
		"the code names the operation that failed")
	assert.Contains(t, got.body, "Couldn't search Plex",
		"the caller is told in plain words, and the log keeps what the PMS said")
}

func TestSearchReportsAMalformedResponse(t *testing.T) {
	t.Parallel()

	ts := mediaTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"MediaContainer":{"Hub":"not-a-list"}}`)
	})

	client, server := pmsFor(t, ts)

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Client().Return(client, server, true)

	got := search(t, New(auth), "/api/plex/search"+"?q=intro")

	assert.Equal(t, fiber.StatusInternalServerError, got.status)
	assert.Contains(t, got.body, "search_failed",
		"a response the server cannot read is a failed search, not an empty one")
}

func TestSearchServesTheMatchingItems(t *testing.T) {
	t.Parallel()

	var gotQuery string

	var gotLibrary string

	ts := mediaTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/hubs/search", r.URL.Path)

		gotQuery = r.URL.Query().Get("query")
		gotLibrary = r.URL.Query().Get("sectionId")

		writeJSON(w, `{"MediaContainer":{"Hub":[
			{"title":"Movies","type":"movie","Metadata":[
				{"ratingKey":"100","title":"Test Movie","type":"movie","year":1999}
			]}
		]}}`)
	})

	client, server := pmsFor(t, ts)

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Client().Return(client, server, true)

	got := search(t, New(auth), "/api/plex/search"+"?q=intro&library=3")

	require.Equal(t, fiber.StatusOK, got.status)
	assert.Equal(t, "intro", gotQuery)
	assert.Equal(t, "3", gotLibrary, "the library filter reaches the PMS")
	assert.JSONEq(t,
		`{"items":[{"id":"100","title":"Test Movie (1999)","type":"movie",
		 "libraryTitle":"Movies","year":1999}],"total":1}`,
		got.body,
		"the hub title becomes the library the hit lives in")
}

func TestSearchOmitsTheSectionWhenNoLibraryIsAsked(t *testing.T) {
	t.Parallel()

	var rawQuery string

	ts := mediaTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery

		writeJSON(w, `{"MediaContainer":{"Hub":[
			{"title":"Movies","type":"movie","Metadata":[
				{"ratingKey":"100","title":"Test Movie","type":"movie","year":1999}
			]}
		]}}`)
	})

	client, server := pmsFor(t, ts)

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Client().Return(client, server, true)

	got := search(t, New(auth), "/api/plex/search"+"?q=intro")

	require.Equal(t, fiber.StatusOK, got.status)
	assert.NotContains(t, rawQuery, "sectionId",
		"searching the whole server must not ask for one section")
}

func TestSearchReportsNoMatchesAsAnEmptyList(t *testing.T) {
	t.Parallel()

	ts := mediaTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"MediaContainer":{"Hub":[]}}`)
	})

	client, server := pmsFor(t, ts)

	auth := mocks.NewMockPlexLookup(t)
	auth.EXPECT().Client().Return(client, server, true)

	got := search(t, New(auth), "/api/plex/search"+"?q=nothing")

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.JSONEq(t, `{"items":[],"total":0}`, got.body,
		"a search that matched nothing is not a failed search")
}
