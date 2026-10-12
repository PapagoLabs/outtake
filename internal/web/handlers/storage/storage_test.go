// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/failure"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/web/respond/respondtest"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// fakeClearer records a preview clear and answers with err.
type fakeClearer struct {
	// err is what the clear reports.
	err error
	// cleared counts the clears.
	cleared int
}

// reply is what one request answered.
type reply struct {
	// header is the response's header.
	header http.Header
	// body is the response's body.
	body string
	// cookies are the cookies it set.
	cookies []*http.Cookie
	// status is its status code.
	status int
}

// errClearFailed stands in for a preview clear that fails.
var errClearFailed = errors.New("clear failed")

// ClearFinished records the clear.
//
// Returns:
//   - removed: Always 1.
//   - err: The fake's err.
func (clearer *fakeClearer) ClearFinished(context.Context) (int, error) {
	clearer.cleared++

	return 1, clearer.err
}

// storageApp mounts the storage routes on a local store.
//
// Parameters:
//   - t: The test the app belongs to.
//   - clearer: The preview clearer, nil for none.
//   - s3: Whether the page reports an S3 bucket.
//
// Returns:
//   - app: The app.
//   - paths: The local layout it measures.
func storageApp(t *testing.T, clearer PreviewClearer, s3 bool) (*fiber.App, blob.Paths) {
	t.Helper()

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	handler := New(store, store.Paths, clearer, s3)

	app := fiber.New()
	respondtest.Sessions(t, app)
	app.Get(routes.PathSettingsStorage, handler.Page)
	app.Post(routes.PathClearPreviews, handler.ClearPreviews)
	app.Post(routes.PathClearThumbnails, handler.ClearThumbnails)

	return app, store.Paths
}

// send serves one request.
//
// Parameters:
//   - t: The test sending it.
//   - app: The app.
//   - method: The request method.
//   - target: The request target.
//
// Returns:
//   - answer: What the app answered.
func send(t *testing.T, app *fiber.App, method, target string) reply {
	t.Helper()

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), method, target, nil))
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return reply{
		header:  resp.Header,
		body:    string(body),
		cookies: resp.Cookies(),
		status:  resp.StatusCode,
	}
}

// writeFiles writes files of the given sizes into a folder.
//
// Parameters:
//   - t: The test the files belong to.
//   - dir: The folder.
//   - sizes: Each file's name and size.
func writeFiles(t *testing.T, dir string, sizes map[string]int) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o750))

	for name, size := range sizes {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o600))
	}
}

// TestPageShowsWhatEachFolderHolds covers the storage page's figures.
func TestPageShowsWhatEachFolderHolds(t *testing.T) {
	t.Parallel()

	app, paths := storageApp(t, nil, false)
	writeFiles(t, paths.ClipsDir(), map[string]int{"a.mp4": 2000, "a.sdr.mp4": 500})
	writeFiles(t, paths.ThumbnailsDir(), map[string]int{"poster.jpg": 1500})

	answer := send(t, app, http.MethodGet, routes.PathSettingsStorage)
	body := answer.body

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Regexp(t, `Video Clips</td>\s*<td[^>]*>2</td>\s*<td[^>]*>2.5 kB</td>`, body)
	assert.Regexp(t, `Thumbnails</td>\s*<td[^>]*>1</td>\s*<td[^>]*>1.5 kB</td>`, body)
	assert.Regexp(t, `Total</td>\s*<td[^>]*>3</td>\s*<td[^>]*>4 kB</td>`, body)
	assert.Contains(t, body, "Clear Previews")
	assert.Contains(t, body, `data-confirm="Delete every cached thumbnail?"`, "a clear asks first")
	assert.Contains(t, body, "data-free-space", "the volume's free space is shown")
	assert.NotContains(t, body, "data-s3-note", "a local store needs no note")
}

// TestPageNotesAnS3Bucket covers the page on S3, whose figures are this
// server's local copies.
func TestPageNotesAnS3Bucket(t *testing.T) {
	t.Parallel()

	app, _ := storageApp(t, nil, true)

	body := send(t, app, http.MethodGet, routes.PathSettingsStorage).body

	assert.Contains(t, body, "These are this server's local copies of the S3 bucket")
}

// TestPageReportsAFolderItCannotRead covers a folder the page cannot list.
//
// The banner reports it, and the page still shows the folders it could read.
func TestPageReportsAFolderItCannotRead(t *testing.T) {
	t.Parallel()

	app, paths := storageApp(t, nil, false)
	require.NoError(t, os.RemoveAll(paths.ClipsDir()))
	require.NoError(t, os.WriteFile(paths.ClipsDir(), []byte("not a folder"), 0o600))
	writeFiles(t, paths.ThumbnailsDir(), map[string]int{"poster.jpg": 1500})

	answer := send(t, app, http.MethodGet, routes.PathSettingsStorage)
	body := answer.body

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, body, "Storage", "the page still renders")
	assert.Contains(t, body, failure.MessageUnexpected, "and the banner reports the failure")
	assert.Regexp(
		t,
		`Thumbnails</td>\s*<td[^>]*>1</td>\s*<td[^>]*>1.5 kB</td>`,
		body,
		"the readable folders are still measured",
	)
}

// TestClearPreviewsClearsAndReturns covers the Clear Previews action.
//
// It clears the previews and returns to the page, and a failure is carried
// there.
func TestClearPreviewsClearsAndReturns(t *testing.T) {
	t.Parallel()

	clearer := &fakeClearer{err: nil, cleared: 0}
	app, _ := storageApp(t, clearer, false)

	answer := send(t, app, http.MethodPost, routes.PathClearPreviews)

	assert.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathSettingsStorage, answer.header.Get(fiber.HeaderLocation))
	assert.Equal(t, 1, clearer.cleared)

	failing := &fakeClearer{err: errClearFailed, cleared: 0}
	failingApp, _ := storageApp(t, failing, false)

	refused := send(t, failingApp, http.MethodPost, routes.PathClearPreviews)

	assert.Equal(t, fiber.StatusSeeOther, refused.status)
	assert.NotEmpty(
		t,
		respondtest.FlashForCookies(t, failingApp, refused.cookies).Message,
		"the page is told it failed",
	)
}

// TestClearPreviewsWithoutAServiceReturns covers a server that renders no
// previews, which has nothing to clear.
func TestClearPreviewsWithoutAServiceReturns(t *testing.T) {
	t.Parallel()

	app, _ := storageApp(t, nil, false)

	answer := send(t, app, http.MethodPost, routes.PathClearPreviews)

	assert.Equal(t, fiber.StatusSeeOther, answer.status)
}

// TestClearThumbnailsEmptiesTheCache covers the Clear Thumbnails action.
func TestClearThumbnailsEmptiesTheCache(t *testing.T) {
	t.Parallel()

	app, paths := storageApp(t, nil, false)
	writeFiles(t, paths.ThumbnailsDir(), map[string]int{"one.jpg": 1, "two.jpg": 1})

	answer := send(t, app, http.MethodPost, routes.PathClearThumbnails)

	assert.Equal(t, fiber.StatusSeeOther, answer.status)

	left, err := blob.StoredFiles(paths.ThumbnailsDir())
	require.NoError(t, err)
	assert.Empty(t, left)
}
