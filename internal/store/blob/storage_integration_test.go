// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

// scratchStore returns a filesystem backend rooted in the test's temp directory.
//
// Parameters:
//   - t: The test that needs the backend.
//
// Returns:
//   - store: A backend with every media directory already created.
func scratchStore(t *testing.T) *blob.Storage {
	t.Helper()

	paths := blob.NewPaths(filepath.Join(t.TempDir(), "output"))

	store, err := blob.NewStorage(paths)
	require.NoError(t, err)

	return store
}

// fakeS3 returns an S3 backend whose endpoint is an in-process fake.
//
// Parameters:
//   - t: The test that needs the backend.
//   - handler: Request handler standing in for an S3 endpoint.
//
// Returns:
//   - store: The S3 backend.
func fakeS3(t *testing.T, handler http.Handler) *blob.S3 {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	paths := blob.NewPaths(filepath.Join(t.TempDir(), "output"))

	store, err := blob.NewS3(&config.Config{
		S3Endpoint:     server.URL,
		S3Bucket:       "test-bucket",
		S3Region:       "us-east-1",
		S3AccessKey:    "test-access-key",
		S3SecretKey:    "test-secret-key",
		S3UsePathStyle: true,
	}, paths)
	require.NoError(t, err)

	return store
}

// writeFile puts bytes at path, failing the test when it cannot.
//
// Parameters:
//   - t: The test that needs the file.
//   - path: File to write.
//   - body: File contents.
func writeFile(t *testing.T, path, body string) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestIntegration_NewStorageCreatesEveryMediaDirectory(t *testing.T) {
	t.Parallel()

	store := scratchStore(t)

	for _, dir := range []string{
		store.ClipsDir(),
		store.GifsDir(),
		store.PreviewsDir(),
		store.ScreenshotsDir(),
		store.ThumbnailsDir(),
	} {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir(), dir+" is created up front")
	}

	for _, dir := range []string{
		store.ClipsDir(),
		store.GifsDir(),
		store.PreviewsDir(),
		store.ScreenshotsDir(),
		store.ThumbnailsDir(),
	} {
		assert.Equal(t, store.BasePath(), filepath.Dir(dir), "every directory sits under the base")
	}
}

func TestIntegration_FilesystemBackendRoundTripsAFile(t *testing.T) {
	t.Parallel()

	store := scratchStore(t)

	target := store.ClipPath("clip-1")

	assert.False(t, store.FileExists(target))

	writeFile(t, target, "rendered clip")

	assert.True(t, store.FileExists(target))

	// The filesystem backend owns the bytes already, so a transfer is a no-op
	// that must leave the file exactly where it is.
	require.NoError(t, store.Put(t.Context(), target))
	require.NoError(t, store.Get(t.Context(), target))

	body, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "rendered clip", string(body))

	require.NoError(t, store.DeleteFile(target))
	assert.False(t, store.FileExists(target))

	require.NoError(t, store.DeleteFile(target), "removing an absent file is not an error")
}

func TestIntegration_ThumbnailIsWrittenUnderItsOwnDirectory(t *testing.T) {
	t.Parallel()

	store := scratchStore(t)

	require.NoError(t, store.WriteThumbnail("clip-1", []byte("thumbnail-bytes")))

	assert.Equal(t, store.ThumbnailsDir(), filepath.Dir(store.ThumbnailPath("clip-1")))

	body, err := os.ReadFile(store.ThumbnailPath("clip-1"))
	require.NoError(t, err)
	assert.Equal(t, "thumbnail-bytes", string(body))

	require.NoError(t, store.WriteThumbnail("clip-1", []byte("replaced")))

	body, err = os.ReadFile(store.ThumbnailPath("clip-1"))
	require.NoError(t, err)
	assert.Equal(t, "replaced", string(body), "a rewritten thumbnail replaces the first")

	require.NoError(t, store.DeleteFile(store.ThumbnailPath("clip-1")))
	assert.False(t, store.FileExists(store.ThumbnailPath("clip-1")))
}

func TestIntegration_OutputPathLaysEachClipTypeOut(t *testing.T) {
	t.Parallel()

	store := scratchStore(t)

	assert.Equal(t, store.ClipPath("clip-1"), store.OutputPath("clip-1", clip.TypeClip))
	assert.Equal(t, store.GifPath("clip-1"), store.OutputPath("clip-1", clip.TypeGIF))
	assert.Equal(
		t,
		store.ScreenshotPath("clip-1"),
		store.OutputPath("clip-1", clip.TypeScreenshot),
	)
	assert.Empty(t, store.OutputPath("clip-1", clip.Type("unknown")),
		"a type this app does not produce has no destination")

	assert.Equal(t, ".mp4", filepath.Ext(store.ClipPath("clip-1")))
	assert.Equal(t, ".gif", filepath.Ext(store.GifPath("clip-1")))
	assert.Equal(t, ".jpg", filepath.Ext(store.ScreenshotPath("clip-1")))
	assert.Equal(t, ".jpg", filepath.Ext(store.ThumbnailPath("clip-1")))
	assert.Equal(t, ".mp4", filepath.Ext(store.PreviewPath("clip-1")))
}

func TestIntegration_S3RejectsPathsOutsideTheScratchDirectory(t *testing.T) {
	t.Parallel()

	var reached bool

	store := fakeS3(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		reached = true

		writer.WriteHeader(http.StatusOK)
	}))

	base := store.Paths().BasePath()

	outside := filepath.Join(filepath.Dir(base), "escape.mp4")
	err := store.Get(t.Context(), outside)
	require.ErrorContains(t, err, "path is outside storage scratch directory")

	err = store.Put(t.Context(), outside)
	require.ErrorContains(t, err, "path is outside storage scratch directory")

	err = store.DeleteFile(outside)
	require.ErrorContains(t, err, "path is outside storage scratch directory")

	err = store.Get(t.Context(), base)
	require.ErrorContains(
		t,
		err,
		"path is outside storage scratch directory",
		"the scratch root itself is not a key",
	)

	err = store.Get(t.Context(), filepath.Join(base, "clips", "..", "..", "escape.mp4"))
	require.ErrorContains(
		t,
		err,
		"path is outside storage scratch directory",
		"a path that cleans back out is refused",
	)

	assert.False(t, store.FileExists(outside))
	assert.False(t, reached, "a refused path never reaches the endpoint")
}

func TestIntegration_S3UploadsDownloadsAndDeletesThroughTheEndpoint(t *testing.T) {
	t.Parallel()

	const objectBody = "remote clip bytes"

	var mu sync.Mutex

	held := false

	store := fakeS3(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch request.Method {
		case http.MethodPut:
			held = true
		case http.MethodHead:
			if !held {
				writer.WriteHeader(http.StatusNotFound)

				return
			}
		case http.MethodGet:
			if !held {
				writer.WriteHeader(http.StatusNotFound)

				return
			}

			_, err := writer.Write([]byte(objectBody))
			assert.NoError(t, err)

			return
		case http.MethodDelete:
			held = false

			writer.WriteHeader(http.StatusNoContent)

			return
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))

	target := store.Paths().ClipPath("clip-1")

	writeFile(t, target, "local only")
	require.NoError(t, store.Put(t.Context(), target))

	require.NoError(t, os.Remove(target))
	assert.True(t, store.FileExists(target),
		"an object held only by the endpoint is hydrated so the local copy exists again")

	require.NoError(t, store.Get(t.Context(), target))

	body, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, objectBody, string(body), "the download landed on the scratch path")

	require.NoError(t, store.DeleteFile(target))
	assert.False(t, store.FileExists(target), "the remote copy and the local one both went")
}

func TestIntegration_S3ReportsAnObjectTheEndpointDoesNotHold(t *testing.T) {
	t.Parallel()

	store := fakeS3(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))

	missing := store.Paths().ClipPath("absent")

	assert.False(t, store.FileExists(missing))

	err := store.Get(t.Context(), missing)
	require.Error(t, err, "a download of an absent object fails")

	require.NoError(t, store.DeleteFile(missing), "deleting an absent object is tolerated")
}
