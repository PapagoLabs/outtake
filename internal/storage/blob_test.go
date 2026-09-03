// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/config"
)

func TestBlob_FilesystemRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewStorage(t.TempDir())
	require.NoError(t, err)

	var blob Blob = store
	path := blob.ClipPath("clip-1")
	require.NoError(t, os.WriteFile(path, []byte("mp4"), filePermissions))
	require.NoError(t, blob.Put(t.Context(), path))
	require.NoError(t, blob.Get(t.Context(), path))
	assert.True(t, blob.FileExists(path))
	require.NoError(t, blob.DeleteFile(path))
	assert.False(t, blob.FileExists(path))
}

func TestNewFromConfig_Filesystem(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewFromConfig(testStorageConfig(dir, "filesystem"))
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(dir, "clips"))
	assert.Contains(t, store.ClipPath("abc"), "clips")
}

func TestNewFromConfig_UnknownBackend(t *testing.T) {
	t.Parallel()

	_, err := NewFromConfig(testStorageConfig(t.TempDir(), "gcs"))
	require.ErrorIs(t, err, errUnknownStorageBackend)
}

func TestNewFromConfig_S3MissingBucket(t *testing.T) {
	t.Parallel()

	cfg := testStorageConfig(t.TempDir(), "s3")
	cfg.S3Endpoint = "http://localhost:8333"
	_, err := NewFromConfig(cfg)
	require.ErrorIs(t, err, errS3BucketRequired)
}

func testStorageConfig(path, backend string) *config.Config {
	return &config.Config{
		ListenAddr:      "",
		DatabasePath:    "",
		DatabaseBackend: "",
		DatabaseURL:     "",
		StoragePath:     path,
		StorageBackend:  backend,
		S3Endpoint:      "",
		S3Bucket:        "",
		S3Region:        "",
		S3AccessKey:     "",
		S3SecretKey:     "",
		S3UsePathStyle:  true,
		FFmpegPath:      "",
		FFprobePath:     "",
		LogLevel:        "",
		Env:             "",
		SessionPollSec:  0,
		NumWorkers:      0,
		MaxClipDurSec:   0,
		CropBlackBars:   false,
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   "",
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}
}
