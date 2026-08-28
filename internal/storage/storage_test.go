// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package storage

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStorage_CreatesDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := NewStorage(filepath.Join(dir, "storage"))
	require.NoError(t, err)

	assert.DirExists(t, s.ClipsDir())
	assert.DirExists(t, s.GifsDir())
	assert.DirExists(t, s.ScreenshotsDir())
	assert.DirExists(t, s.ThumbnailsDir())
}

func TestStorage_Paths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := NewStorage(filepath.Join(dir, "output"))
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "output", "clips", "abc.mp4"), s.ClipPath("abc"))
	assert.Equal(t, filepath.Join(dir, "output", "gifs", "abc.gif"), s.GifPath("abc"))
	assert.Equal(t, filepath.Join(dir, "output", "screenshots", "abc.jpg"), s.ScreenshotPath("abc"))
	assert.Equal(t, filepath.Join(dir, "output", "thumbnails", "abc.jpg"), s.ThumbnailPath("abc"))
}

func TestStorage_WriteThumbnail(t *testing.T) {
	t.Parallel()

	store, err := NewStorage(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.WriteThumbnail("abc", []byte("jpeg")))
	assert.True(t, store.FileExists(store.ThumbnailPath("abc")))
}

func TestStorage_FileExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := NewStorage(dir)
	require.NoError(t, err)

	assert.False(t, s.FileExists(filepath.Join(dir, "nonexistent")))
}

func TestStorage_DeleteFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := NewStorage(dir)
	require.NoError(t, err)

	err = s.DeleteFile(filepath.Join(dir, "nonexistent"))
	require.NoError(t, err)
}

func TestStorage_BasePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := NewStorage(filepath.Join(dir, "output"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "output"), s.BasePath())
}
