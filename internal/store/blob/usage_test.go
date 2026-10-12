// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusingStore deletes like a local store but refuses one file.
type refusingStore struct {
	*Storage

	// refused is the file whose delete fails.
	refused string
}

// errDeleteRefused stands in for a storage that refuses a delete.
var errDeleteRefused = errors.New("delete refused")

// DeleteFile refuses the one file and deletes the rest.
//
// Parameters:
//   - path: The file to delete.
//
// Returns:
//   - err: errDeleteRefused for the refused file.
func (store refusingStore) DeleteFile(path string) error {
	if path == store.refused {
		return errDeleteRefused
	}

	//nolint:wrapcheck // The test reads the local store's own error.
	return store.Storage.DeleteFile(path)
}

// writeStored writes files of the given sizes into a folder.
//
// Parameters:
//   - t: The test the files belong to.
//   - dir: The folder.
//   - files: Each file's name and size.
func writeStored(t *testing.T, dir string, files map[string]int) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o750))

	for name, size := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o600))
	}
}

// TestUsageMeasuresEachFolder covers what the storage page reports.
//
// Each folder counts only its finished files. A staging file and a GIF
// palette are left out, and a folder nothing has written reads as empty.
func TestUsageMeasuresEachFolder(t *testing.T) {
	t.Parallel()

	paths := NewPaths(t.TempDir())

	writeStored(
		t,
		paths.ClipsDir(),
		map[string]int{"a.mp4": 100, "a.sdr.mp4": 40, ".staging-b.mp4": 999},
	)
	writeStored(t, paths.GifsDir(), map[string]int{"c.gif": 30, ".staging-d.palette.png": 7})
	writeStored(t, paths.PreviewsDir(), map[string]int{"p1.mp4": 50})

	usage, err := paths.Usage()
	require.NoError(t, err)

	assert.Equal(t, FolderUsage{Files: 2, Bytes: 140}, usage.Clips, "a clip and its SDR version")
	assert.Equal(t, FolderUsage{Files: 1, Bytes: 30}, usage.GIFs)
	assert.Equal(
		t,
		FolderUsage{Files: 0, Bytes: 0},
		usage.Screenshots,
		"a folder nothing wrote is empty",
	)
	assert.Equal(t, FolderUsage{Files: 1, Bytes: 50}, usage.Previews)
	assert.Equal(t, FolderUsage{Files: 0, Bytes: 0}, usage.Thumbnails)
	assert.Equal(t, FolderUsage{Files: 4, Bytes: 220}, usage.Total())
}

// TestClearThumbnailsDeletesTheCache covers clearing the poster cache.
//
// Every poster goes, and a delete that fails is reported without stopping
// the others.
func TestClearThumbnailsDeletesTheCache(t *testing.T) {
	t.Parallel()

	paths := NewPaths(t.TempDir())
	writeStored(
		t,
		paths.ThumbnailsDir(),
		map[string]int{"one.jpg": 1, "two.jpg": 1, "three.jpg": 1},
	)

	local, err := NewStorage(paths)
	require.NoError(t, err)

	store := refusingStore{Storage: local, refused: filepath.Join(paths.ThumbnailsDir(), "two.jpg")}

	removed, err := ClearThumbnails(t.Context(), store, paths)
	require.ErrorIs(t, err, errDeleteRefused)
	assert.Equal(t, 2, removed, "the other posters still go")

	left, err := StoredFiles(paths.ThumbnailsDir())
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(paths.ThumbnailsDir(), "two.jpg")}, left)
}
