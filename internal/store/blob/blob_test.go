// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlob_FilesystemRoundTrip(t *testing.T) {
	t.Parallel()

	paths := NewPaths(t.TempDir())

	store, err := NewStorage(paths)
	require.NoError(t, err)

	var blob Blob = store

	path := paths.ClipPath("clip-1")
	require.NoError(t, os.WriteFile(path, []byte("mp4"), filePermissions))
	require.NoError(t, blob.Put(t.Context(), path))
	require.NoError(t, blob.Get(t.Context(), path))
	assert.True(t, blob.FileExists(path))
	require.NoError(t, blob.DeleteFile(path))
	assert.False(t, blob.FileExists(path))
}

func TestNewStorage_RejectsUnwritableBase(t *testing.T) {
	t.Parallel()

	blocked := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), filePermissions))

	_, err := NewStorage(NewPaths(blocked))
	require.Error(t, err)
}
