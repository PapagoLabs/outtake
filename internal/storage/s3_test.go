// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package storage

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
)

type fakeS3Server struct {
	mu      sync.Mutex
	objects map[string][]byte
}

const (
	testS3Bucket    = "outtake"
	testS3AccessKey = "key"
	testS3SecretKey = "secret"
)

func TestS3_PutGetDelete(t *testing.T) {
	t.Parallel()

	server := newFakeS3Server()
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	store, err := newS3(s3Settings{
		endpoint:     httpServer.URL,
		bucket:       testS3Bucket,
		region:       defaultS3Region,
		accessKey:    testS3AccessKey,
		secretKey:    testS3SecretKey,
		scratch:      t.TempDir(),
		usePathStyle: true,
	})
	require.NoError(t, err)

	path := store.ClipPath("clip-1")
	require.NoError(t, os.WriteFile(path, []byte("video"), filePermissions))
	require.NoError(t, store.Put(t.Context(), path))

	require.NoError(t, os.Remove(path))
	assert.False(t, store.fs.FileExists(path))
	assert.True(t, store.FileExists(path))
	assert.True(t, store.fs.FileExists(path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("video"), got)

	require.NoError(t, store.DeleteFile(path))
	assert.False(t, store.FileExists(path))
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

	store, err := newS3(s3Settings{
		endpoint:     endpoint,
		bucket:       bucket,
		region:       defaultS3Region,
		accessKey:    os.Getenv("OUTTAKE_S3_ACCESS_KEY"),
		secretKey:    os.Getenv("OUTTAKE_S3_SECRET_KEY"),
		scratch:      t.TempDir(),
		usePathStyle: true,
	})
	require.NoError(t, err)

	path := store.ClipPath("live-clip")
	require.NoError(t, os.WriteFile(path, []byte("video"), filePermissions))
	require.NoError(t, store.Put(t.Context(), path))
	require.NoError(t, store.DeleteFile(path))
}

func TestS3_WriteThumbnail(t *testing.T) {
	t.Parallel()

	server := newFakeS3Server()
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	store, err := newS3(s3Settings{
		endpoint:     httpServer.URL,
		bucket:       testS3Bucket,
		region:       defaultS3Region,
		accessKey:    testS3AccessKey,
		secretKey:    testS3SecretKey,
		scratch:      t.TempDir(),
		usePathStyle: true,
	})
	require.NoError(t, err)
	require.NoError(t, store.WriteThumbnail("abc", []byte("jpeg")))
	assert.True(t, store.FileExists(store.ThumbnailPath("abc")))
}

func TestNewFromConfig_S3(t *testing.T) {
	t.Parallel()

	server := newFakeS3Server()
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)

	cfg := testStorageConfig(t.TempDir(), "s3")

	cfg.S3Endpoint = httpServer.URL
	cfg.S3Bucket = testS3Bucket
	cfg.S3AccessKey = testS3AccessKey
	cfg.S3SecretKey = testS3SecretKey

	store, err := NewFromConfig(cfg)
	require.NoError(t, err)
	assert.Contains(t, store.GifPath("g1"), "gifs")
}

func TestNewFromConfig_S3MissingEndpoint(t *testing.T) {
	t.Parallel()

	cfg := testStorageConfig(t.TempDir(), "s3")

	cfg.S3Bucket = testS3Bucket

	_, err := NewFromConfig(cfg)
	require.ErrorIs(t, err, errS3EndpointRequired)
}

func newFakeS3Server() *fakeS3Server {
	return &fakeS3Server{
		mu:      sync.Mutex{},
		objects: map[string][]byte{},
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

func (server *fakeS3Server) getObject(writer http.ResponseWriter, key string) {
	server.mu.Lock()

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
