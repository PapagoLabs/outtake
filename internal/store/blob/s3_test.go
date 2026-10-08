// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/settings/config"
)

type fakeS3Server struct {
	mu      sync.Mutex
	objects map[string][]byte
	// gets counts object downloads, so a test can tell a lookup from a fetch.
	gets int
}

func TestS3_PutGetDelete(t *testing.T) {
	t.Parallel()

	server := newFakeS3Server()
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	store, err := newS3FromSettings(s3Settings{
		endpoint:     httpServer.URL,
		bucket:       "outtake",
		region:       defaultS3Region,
		accessKey:    "key",
		secretKey:    "secret",
		scratch:      NewPaths(t.TempDir()),
		usePathStyle: true,
	})
	require.NoError(t, err)

	path := store.Paths().ClipPath("clip-1")
	require.NoError(t, os.WriteFile(path, []byte("video"), filePermissions))
	require.NoError(t, store.Put(t.Context(), path))

	require.NoError(t, os.Remove(path))
	assert.True(t, store.Exists(t.Context(), path), "the object is found on S3")
	assert.False(t, store.fs.Exists(t.Context(), path), "without being downloaded")
	assert.Zero(t, server.downloads())

	require.NoError(t, store.Ensure(t.Context(), path))
	require.NoError(t, store.Ensure(t.Context(), path))
	assert.Equal(t, 1, server.downloads(), "a local copy is fetched once and then reused")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("video"), got)

	require.NoError(t, store.DeleteFile(path))
	assert.False(t, store.Exists(t.Context(), path))
	require.Error(t, store.Ensure(t.Context(), path), "an object that is gone cannot be served")
}

func TestS3_SkipWithoutEndpoint(t *testing.T) {
	t.Parallel()

	endpoint := os.Getenv("OUTTAKE_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("OUTTAKE_S3_ENDPOINT not set")
	}

	bucket := os.Getenv("OUTTAKE_S3_BUCKET")
	if bucket == "" {
		t.Skip("OUTTAKE_S3_BUCKET not set")
	}

	store, err := newS3FromSettings(s3Settings{
		endpoint:     endpoint,
		bucket:       bucket,
		region:       defaultS3Region,
		accessKey:    os.Getenv("OUTTAKE_S3_ACCESS_KEY"),
		secretKey:    os.Getenv("OUTTAKE_S3_SECRET_KEY"),
		scratch:      NewPaths(t.TempDir()),
		usePathStyle: true,
	})
	require.NoError(t, err)

	path := store.Paths().ClipPath("live-clip")
	require.NoError(t, os.WriteFile(path, []byte("video"), filePermissions))
	require.NoError(t, store.Put(t.Context(), path))
	require.NoError(t, store.DeleteFile(path))
}

func TestS3_WriteThumbnail(t *testing.T) {
	t.Parallel()

	server := newFakeS3Server()
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	store, err := newS3FromSettings(s3Settings{
		endpoint:     httpServer.URL,
		bucket:       "outtake",
		region:       defaultS3Region,
		accessKey:    "key",
		secretKey:    "secret",
		scratch:      NewPaths(t.TempDir()),
		usePathStyle: true,
	})
	require.NoError(t, err)
	require.NoError(t, store.WriteThumbnail("abc", []byte("jpeg")))
	assert.True(t, store.Exists(t.Context(), store.Paths().ThumbnailPath("abc")))
}

func TestS3_SharesTheScratchLayout(t *testing.T) {
	t.Parallel()

	server := newFakeS3Server()
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	paths := NewPaths(t.TempDir())

	store, err := NewS3(testS3Config(httpServer.URL), paths)
	require.NoError(t, err)

	assert.Equal(t, paths, store.Paths())
	assert.Equal(t, paths.GifPath("g1"), store.Paths().GifPath("g1"))
}

func TestNewS3_MissingBucket(t *testing.T) {
	t.Parallel()

	cfg := testS3Config("http://localhost:8333")

	cfg.S3Bucket = ""

	_, err := NewS3(cfg, NewPaths(t.TempDir()))
	require.ErrorIs(t, err, errS3BucketRequired)
}

func TestNewS3_MissingEndpoint(t *testing.T) {
	t.Parallel()

	_, err := NewS3(testS3Config(""), NewPaths(t.TempDir()))
	require.ErrorIs(t, err, errS3EndpointRequired)
}

func testS3Config(endpoint string) *config.Config {
	return &config.Config{
		S3Endpoint:     endpoint,
		S3Bucket:       "outtake",
		S3AccessKey:    "key",
		S3SecretKey:    "secret",
		S3UsePathStyle: true,
	}
}

func newFakeS3Server() *fakeS3Server {
	return &fakeS3Server{
		mu:      sync.Mutex{},
		objects: map[string][]byte{},
		gets:    0,
	}
}

func (server *fakeS3Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	_, key := splitS3Path(request.URL.Path)
	switch request.Method {
	case http.MethodPut:
		server.putObject(writer, request, key)
	case http.MethodGet:
		server.getObject(writer, key)
	case http.MethodHead:
		server.headObject(writer, key)
	case http.MethodDelete:
		server.deleteObject(writer, key)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (server *fakeS3Server) deleteObject(writer http.ResponseWriter, key string) {
	server.mu.Lock()
	delete(server.objects, key)
	server.mu.Unlock()
	writer.WriteHeader(http.StatusNoContent)
}

// downloads reports how many objects were downloaded.
//
// Returns:
//   - count: Object GET requests served.
func (server *fakeS3Server) downloads() int {
	server.mu.Lock()
	defer server.mu.Unlock()

	return server.gets
}

func (server *fakeS3Server) getObject(writer http.ResponseWriter, key string) {
	server.mu.Lock()

	server.gets++

	data, ok := server.objects[key]

	server.mu.Unlock()

	if !ok {
		writeS3NotFound(writer)

		return
	}

	writer.Header().Set("Content-Length", strconv.Itoa(len(data)))
	writer.WriteHeader(http.StatusOK)

	_, _ = writer.Write(data)
}

func (server *fakeS3Server) headObject(writer http.ResponseWriter, key string) {
	server.mu.Lock()

	data, ok := server.objects[key]

	server.mu.Unlock()

	if !ok {
		writeS3NotFound(writer)

		return
	}

	writer.Header().Set("Content-Length", strconv.Itoa(len(data)))
	writer.WriteHeader(http.StatusOK)
}

func (server *fakeS3Server) putObject(
	writer http.ResponseWriter,
	request *http.Request,
	key string,
) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)

		return
	}

	server.mu.Lock()

	server.objects[key] = body
	server.mu.Unlock()
	writer.WriteHeader(http.StatusOK)
}

func splitS3Path(urlPath string) (string, string) {
	trimmed := strings.TrimPrefix(urlPath, "/")
	bucket, key, found := strings.Cut(trimmed, "/")
	if !found {
		return bucket, ""
	}

	return bucket, key
}

func writeS3NotFound(writer http.ResponseWriter) {
	writer.WriteHeader(http.StatusNotFound)

	_, _ = writer.Write([]byte(
		`<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code></Error>`,
	))
}
