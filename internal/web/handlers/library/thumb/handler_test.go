// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package thumb

import (
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	storagemocks "github.com/PapagoLabs/outtake/internal/store/blob/mocks"
	"github.com/PapagoLabs/outtake/internal/web/handlers/library/thumb/mocks"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

type thumbAnswer struct {
	status   int
	body     string
	cacheCtl string
	ctype    string
}

// testThumbPath is the Plex thumbnail path the tests request.
const testThumbPath = "/library/metadata/100/thumb/1"

// errStoreClosed reports a blob store that cannot be written to.
var errStoreClosed = errors.New("object store is closed")

// thumbAnswer is what one served thumbnail request produced.

// cacheIDFor returns the cache id a server's testThumbPath is filed under.
//
// Parameters:
//   - server: The Plex server the thumbnail comes from.
//
// Returns:
//   - id: The cache id.
func cacheIDFor(server plex.Server) string {
	return library.CacheID(plex.SelectionKey(server), testThumbPath)
}

// boundServer returns a selection bound to a server no request reaches.
//
// Parameters:
//   - t: The test the selection belongs to.
//
// Returns:
//   - selected: A selection answering with the server.
//   - server: The bound server.
func boundServer(t *testing.T) (*mocks.MockServerSelection, plex.Server) {
	t.Helper()

	server := plex.Server{
		Name:      "Attic",
		Address:   "127.0.0.1",
		Port:      1,
		Scheme:    "http",
		MachineID: "machine-1",
	}

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(plex.NewClient(plex.ClientConfig{}), server, true)

	return selected, server
}

// seedCacheFile writes a cache file the handler can send.
//
// Parameters:
//   - t: The test the file belongs to.
//   - cached: Local path to write.
//   - contents: Bytes to write.
func seedCacheFile(t *testing.T, cached, contents string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(cached), 0o750))
	require.NoError(t, os.WriteFile(cached, []byte(contents), 0o600))
}

// thumbServer starts a loopback server standing in for a PMS.
//
// Parameters:
//   - t: The test the server belongs to.
//   - status: HTTP status to answer with.
//   - contentType: Content type to report, empty to report none.
//   - body: Body to answer with.
//
// Returns:
//   - ts: The loopback server, closed when the test ends.
func thumbServer(t *testing.T, status int, contentType, body string) *httptest.Server {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set(fiber.HeaderContentType, contentType)
		}

		w.WriteHeader(status)

		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)

	return ts
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

	return plex.NewClient(plex.ClientConfig{
		Product: "outtake-test",
		Timeout: 5 * time.Second,
	}), plex.Server{
		Name:    "loopback",
		Address: host,
		Port:    portNumber,
		Scheme:  "http",
	}
}

// getThumb serves one thumbnail request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - query: Query string, without the leading question mark.
//
// Returns:
//   - thumbAnswer: The status, body, and headers the handler wrote.
func getThumb(t *testing.T, handler *Handler, query string) thumbAnswer {
	t.Helper()

	app := fiber.New()
	app.Get(routes.PathThumb, handler.Get)

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, routes.PathThumb+"?"+query, nil,
	)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return thumbAnswer{
		status:   resp.StatusCode,
		body:     string(body),
		cacheCtl: resp.Header.Get("Cache-Control"),
		ctype:    resp.Header.Get(fiber.HeaderContentType),
	}
}

// thumbQuery builds the query naming one Plex thumbnail path.
//
// Parameters:
//   - thumbPath: Plex thumbnail path to request.
//
// Returns:
//   - query: The encoded query string.
func thumbQuery(thumbPath string) string {
	return routes.QueryThumbPath + "=" + url.QueryEscape(thumbPath)
}

func TestNewKeepsTheCollaboratorsItWasGiven(t *testing.T) {
	t.Parallel()

	store := storagemocks.NewMockBlob(t)
	selected := mocks.NewMockServerSelection(t)
	paths := blob.NewPaths("/data")

	assert.Equal(t,
		&Handler{store: store, paths: paths, selected: selected},
		New(store, paths, selected),
	)
}

func TestGetRejectsAPathThatIsNotALibraryAsset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
	}{
		{name: "no path at all", give: ""},
		{name: "a relative path", give: "library/metadata/100/thumb/1"},
		{name: "a traversal out of the library route", give: "/library/../../etc/passwd"},
		{name: "an unrelated absolute route", give: "/status/sessions"},
		{name: "a file scheme", give: "file:///etc/passwd"},
		{name: "a media file", give: "/library/parts/1/1700000000/file.mkv"},
		{name: "a library refresh", give: "/library/sections/1/refresh"},
		{name: "the photo transcoder", give: "/photo/:/transcode?url=http://example.com/x.jpg"},
		{name: "artwork with a query string", give: "/library/metadata/1/thumb/2?url=x"},
		{name: "an escaped path", give: "/library/metadata/1%2F..%2Fparts/thumb/2"},
		{name: "a theme song", give: "/library/metadata/1/theme/2"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := getThumb(t, New(
				storagemocks.NewMockBlob(t),
				blob.NewPaths(t.TempDir()),
				mocks.NewMockServerSelection(t),
			), thumbQuery(test.give))

			assert.Equal(t, fiber.StatusBadRequest, got.status,
				"the guard runs before any store or Plex call is made")
		})
	}
}

func TestGetRejectsAnEmptyPathParameter(t *testing.T) {
	t.Parallel()

	got := getThumb(t, New(
		storagemocks.NewMockBlob(t),
		blob.NewPaths(t.TempDir()),
		mocks.NewMockServerSelection(t),
	), routes.QueryThumbPath+"=")

	assert.Equal(t, fiber.StatusBadRequest, got.status)
}

func TestGetServesTheCachedThumbnail(t *testing.T) {
	t.Parallel()

	selected, server := boundServer(t)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(cacheIDFor(server))
	seedCacheFile(t, cached, "cached-bytes")

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Ensure(mock.Anything, cached).Return(nil)

	got := getThumb(t, New(store, paths, selected), thumbQuery(testThumbPath))

	require.Equal(t, fiber.StatusOK, got.status)
	assert.Equal(t, "cached-bytes", got.body)
	assert.Equal(t, library.CacheControl, got.cacheCtl,
		"a cached thumbnail is safe for the browser to hold for a week")
}

func TestGetKeepsEachServersThumbnailsApart(t *testing.T) {
	t.Parallel()

	selected, server := boundServer(t)

	other := server

	other.MachineID = "machine-2"

	paths := blob.NewPaths(t.TempDir())
	seedCacheFile(t, paths.ThumbnailPath(cacheIDFor(other)), "other-server")

	cached := paths.ThumbnailPath(cacheIDFor(server))
	seedCacheFile(t, cached, "this-server")

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Ensure(mock.Anything, cached).Return(nil)

	got := getThumb(t, New(store, paths, selected), thumbQuery(testThumbPath))

	assert.Equal(t, "this-server", got.body, "a poster cached for another server is not served")
}

func TestGetFetchesAndCachesTheThumbnail(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusOK, "image/png", "thumb-bytes")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(cacheIDFor(server))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Ensure(mock.Anything, cached).Return(fs.ErrNotExist)
	store.EXPECT().WriteThumbnail(cacheIDFor(server), []byte("thumb-bytes")).
		RunAndReturn(func(string, []byte) error {
			seedCacheFile(t, cached, "fetched-bytes")

			return nil
		})

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery(testThumbPath))

	require.Equal(t, fiber.StatusOK, got.status)
	assert.Equal(t, "fetched-bytes", got.body,
		"the file the cache wrote is served, without fetching it back from storage")
	assert.Equal(t, library.CacheControl, got.cacheCtl)
}

func TestGetFetchesWhenTheCacheCannotBeRead(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusOK, "image/png", "thumb-bytes")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(cacheIDFor(server))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Ensure(mock.Anything, cached).Return(errStoreClosed)
	store.EXPECT().WriteThumbnail(cacheIDFor(server), []byte("thumb-bytes")).
		Return(errStoreClosed)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery(testThumbPath))

	require.Equal(t, fiber.StatusOK, got.status, "an unreadable cache is a miss, not a failure")
	assert.Equal(t, "thumb-bytes", got.body)
}

func TestGetFallsBackToTheFetchedBytesWhenTheCacheCannotBeWritten(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusOK, "image/png", "thumb-bytes")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(cacheIDFor(server))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Ensure(mock.Anything, cached).Return(fs.ErrNotExist)
	store.EXPECT().WriteThumbnail(cacheIDFor(server), []byte("thumb-bytes")).
		Return(errStoreClosed)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery(testThumbPath))

	require.Equal(t, fiber.StatusOK, got.status,
		"the poster still renders even though nothing was cached")
	assert.Equal(t, "thumb-bytes", got.body, "the bytes Plex sent are served directly")
	assert.Equal(t, "image/png", got.ctype)
	assert.Equal(t, library.CacheControl, got.cacheCtl,
		"the browser cache header is set on the direct send too")
}

func TestGetRejectsARequestWithNoServerBound(t *testing.T) {
	t.Parallel()

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(nil, plex.EmptyServer(), false)

	got := getThumb(t, New(storagemocks.NewMockBlob(t), blob.NewPaths(t.TempDir()), selected),
		thumbQuery(testThumbPath))

	assert.Equal(t, fiber.StatusBadRequest, got.status,
		"the cache is keyed by server, so nothing is looked up without one")
}

func TestGetReportsAMissingThumbnailOnTheServer(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusNotFound, "", "")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(cacheIDFor(server))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Ensure(mock.Anything, cached).Return(fs.ErrNotExist)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery(testThumbPath))

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"an asset the server does not have is not found")
}

func TestGetReportsACachedFileThatVanished(t *testing.T) {
	t.Parallel()

	selected, server := boundServer(t)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(cacheIDFor(server))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Ensure(mock.Anything, cached).Return(nil)

	got := getThumb(t, New(store, paths, selected), thumbQuery(testThumbPath))

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"the store promised a file it cannot send, so the send fails rather than "+
			"silently answering with something else")
}

func TestGetReportsAFetchedThumbnailTheStoreDidNotLeaveOnDisk(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusOK, "image/png", "thumb-bytes")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(cacheIDFor(server))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Ensure(mock.Anything, cached).Return(fs.ErrNotExist)
	store.EXPECT().
		WriteThumbnail(cacheIDFor(server), []byte("thumb-bytes")).
		Return(nil)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery(testThumbPath))

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"a fetch that cannot be written or sent is reported, not silently dropped")
}
