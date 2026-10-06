// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package thumb

import (
	"errors"
	"io"
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

// errStoreClosed reports a blob store that cannot be written to.
var errStoreClosed = errors.New("object store is closed")

// thumbAnswer is what one served thumbnail request produced.

// seedThumbPath writes a cached thumbnail file the handler can serve.
//
// Parameters:
//   - t: The test the thumbnail belongs to.
//   - paths: Path layout the handler reads the cache through.
//   - thumbPath: Plex thumbnail path the cache entry stands in for.
//
// Returns:
//   - cached: The local path that was written.
func seedThumbPath(t *testing.T, paths blob.Paths, thumbPath string) string {
	t.Helper()

	cached := paths.ThumbnailPath(library.CacheID(thumbPath))
	seedCacheFile(t, cached, "cached-bytes")

	return cached
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

	paths := blob.NewPaths(t.TempDir())
	cached := seedThumbPath(t, paths, "/library/metadata/100/thumb/1")

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().FileExists(cached).Return(true)
	store.EXPECT().Get(mock.Anything, cached).Return(nil)

	got := getThumb(t, New(store, paths, mocks.NewMockServerSelection(t)),
		thumbQuery("/library/metadata/100/thumb/1"))

	require.Equal(t, fiber.StatusOK, got.status)
	assert.Equal(t, "cached-bytes", got.body)
	assert.Equal(t, library.CacheControl, got.cacheCtl,
		"a cached thumbnail is safe to hold for a week")
}

func TestGetReportsACacheReadFailure(t *testing.T) {
	t.Parallel()

	paths := blob.NewPaths(t.TempDir())
	cached := seedThumbPath(t, paths, "/library/metadata/100/thumb/1")

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().FileExists(cached).Return(true)
	store.EXPECT().Get(mock.Anything, cached).Return(errStoreClosed)

	got := getThumb(t, New(store, paths, mocks.NewMockServerSelection(t)),
		thumbQuery("/library/metadata/100/thumb/1"))

	assert.Equal(t, fiber.StatusInternalServerError, got.status,
		"a cache entry that cannot be downloaded is a server failure, not a miss")
}

func TestGetFetchesAndCachesTheThumbnail(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusOK, "image/png", "thumb-bytes")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(library.CacheID("/library/metadata/100/thumb/1"))
	seedCacheFile(t, cached, "fetched-bytes")

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().FileExists(cached).Return(false)
	store.EXPECT().WriteThumbnail(library.CacheID("/library/metadata/100/thumb/1"),
		[]byte("thumb-bytes")).Return(nil)
	store.EXPECT().Get(mock.Anything, cached).Return(nil)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery("/library/metadata/100/thumb/1"))

	require.Equal(t, fiber.StatusOK, got.status)
	assert.Equal(t, "fetched-bytes", got.body, "the freshly cached file is what is served")
	assert.Equal(t, library.CacheControl, got.cacheCtl)
}

func TestGetFallsBackToTheFetchedBytesWhenTheCacheCannotBeWritten(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusOK, "image/png", "thumb-bytes")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(library.CacheID("/library/metadata/100/thumb/1"))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().FileExists(cached).Return(false)
	store.EXPECT().WriteThumbnail(library.CacheID("/library/metadata/100/thumb/1"),
		[]byte("thumb-bytes")).Return(errStoreClosed)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery("/library/metadata/100/thumb/1"))

	require.Equal(t, fiber.StatusOK, got.status,
		"the poster still renders even though nothing was cached")
	assert.Equal(t, "thumb-bytes", got.body, "the bytes Plex sent are served directly")
	assert.Equal(t, "image/png", got.ctype)
	assert.Equal(t, library.CacheControl, got.cacheCtl,
		"the browser cache header is set on the direct send too")
}

func TestGetFallsBackToTheFetchedBytesWhenTheCacheCannotBeRead(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusOK, "image/png", "thumb-bytes")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(library.CacheID("/library/metadata/100/thumb/1"))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().FileExists(cached).Return(false)
	store.EXPECT().WriteThumbnail(library.CacheID("/library/metadata/100/thumb/1"),
		[]byte("thumb-bytes")).Return(nil)
	store.EXPECT().Get(mock.Anything, cached).Return(errStoreClosed)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery("/library/metadata/100/thumb/1"))

	require.Equal(t, fiber.StatusOK, got.status)
	assert.Equal(t, "thumb-bytes", got.body)
	assert.Equal(t, "image/png", got.ctype)
	assert.Equal(t, library.CacheControl, got.cacheCtl)
}

func TestGetRejectsAFetchWithNoServerBound(t *testing.T) {
	t.Parallel()

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(library.CacheID("/library/metadata/100/thumb/1"))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().FileExists(cached).Return(false)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(nil, plex.EmptyServer(), false)

	got := getThumb(t, New(store, paths, selected), thumbQuery("/library/metadata/100/thumb/1"))

	assert.Equal(t, fiber.StatusBadRequest, got.status)
}

func TestGetReportsAMissingThumbnailOnTheServer(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusNotFound, "", "")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(library.CacheID("/library/metadata/100/thumb/1"))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().FileExists(cached).Return(false)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery("/library/metadata/100/thumb/1"))

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"an asset the server does not have is not found")
}

func TestGetReportsACachedFileThatVanished(t *testing.T) {
	t.Parallel()

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(library.CacheID("/library/metadata/100/thumb/1"))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().FileExists(cached).Return(true)
	store.EXPECT().Get(mock.Anything, cached).Return(nil)

	got := getThumb(t, New(store, paths, mocks.NewMockServerSelection(t)),
		thumbQuery("/library/metadata/100/thumb/1"))

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"the store promised a file it cannot send, so the send fails rather than "+
			"silently answering with something else")
}

func TestGetReportsAFetchedThumbnailTheStoreCouldNotWrite(t *testing.T) {
	t.Parallel()

	ts := thumbServer(t, http.StatusOK, "image/png", "thumb-bytes")
	client, server := pmsFor(t, ts)

	paths := blob.NewPaths(t.TempDir())
	cached := paths.ThumbnailPath(library.CacheID("/library/metadata/100/thumb/1"))

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().FileExists(cached).Return(false)
	store.EXPECT().WriteThumbnail(library.CacheID("/library/metadata/100/thumb/1"),
		[]byte("thumb-bytes")).Return(nil)
	store.EXPECT().Get(mock.Anything, cached).Return(nil)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(client, server, true)

	got := getThumb(t, New(store, paths, selected), thumbQuery("/library/metadata/100/thumb/1"))

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"a fetch that cannot be written or sent is reported, not silently dropped")
}
