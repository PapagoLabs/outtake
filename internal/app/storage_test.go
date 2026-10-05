// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

func TestInitStorage_Filesystem(t *testing.T) {
	t.Parallel()

	cfg := testAppConfig(t, "")

	cfg.StorageBackend = blob.BackendFilesystem

	store, err := initStorage(cfg)
	require.NoError(t, err)

	assert.DirExists(t, filepath.Join(cfg.StoragePath, "clips"))
	assert.Contains(t, store.paths.ClipPath("abc"), "clips")
}

func TestInitStorage_EmptyBackendIsFilesystem(t *testing.T) {
	t.Parallel()

	cfg := testAppConfig(t, "")

	cfg.StorageBackend = ""

	store, err := initStorage(cfg)
	require.NoError(t, err)

	assert.Equal(t, blob.NewPaths(cfg.StoragePath), store.paths)
}

func TestInitStorage_UnknownBackend(t *testing.T) {
	t.Parallel()

	cfg := testAppConfig(t, "")

	cfg.StorageBackend = "gcs"

	_, err := initStorage(cfg)
	require.ErrorIs(t, err, blob.ErrUnknownStorageBackend)
}

func TestInitStorage_S3(t *testing.T) {
	t.Parallel()

	cfg := testAppConfig(t, "")

	cfg.StorageBackend = " S3 "
	cfg.S3Endpoint = "http://localhost:8333"
	cfg.S3Bucket = "outtake"

	store, err := initStorage(cfg)
	require.NoError(t, err)

	assert.Equal(t, blob.NewPaths(cfg.StoragePath), store.paths)
	assert.NotNil(t, store.blob)
}

func TestInitStorage_S3IncompleteConfig(t *testing.T) {
	t.Parallel()

	cfg := testAppConfig(t, "")

	cfg.StorageBackend = blob.BackendS3

	_, err := initStorage(cfg)
	require.ErrorContains(t, err, "s3-bucket is required")
}

func TestInitStorage_ReportsBackendFailure(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{StorageBackend: blob.BackendFilesystem, StoragePath: t.TempDir()}

	blocked := filepath.Join(cfg.StoragePath, "blocked")

	cfg.StoragePath = filepath.Join(blocked, "output")

	require.NoError(t, writeFile(blocked))

	_, err := initStorage(cfg)
	require.ErrorContains(t, err, "filesystem storage")
}

func writeFile(path string) error {
	err := os.WriteFile(path, []byte("x"), 0o644)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
