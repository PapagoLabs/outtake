// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package blob provides blob storage for media files.
package blob

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Storage is the local filesystem implementation of the [Blob] port.
type Storage struct {
	Paths
}

const (
	// dirPermissions is the permission directories are created with.
	dirPermissions = 0o755

	// filePermissions is the permission files are created with.
	filePermissions = 0o644
)

// NewStorage creates a storage backend over paths.
//
// Parameters:
//   - paths: The local media layout the backend serves.
//
// Returns:
//   - store: A storage backend rooted at paths.
//   - err: Non-nil when a directory cannot be created.
func NewStorage(paths Paths) (*Storage, error) {
	err := os.MkdirAll(paths.BasePath(), dirPermissions)
	if err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}

	for _, sub := range mediaDirs() {
		dir := filepath.Join(paths.BasePath(), sub)
		err := os.MkdirAll(dir, dirPermissions)
		if err != nil {
			return nil, fmt.Errorf("create %s dir: %w", sub, err)
		}
	}

	return &Storage{Paths: paths}, nil
}

// DeleteFile deletes a file, ignoring one that is already gone.
//
// Parameters:
//   - path: The file to remove.
//
// Returns:
//   - err: Non-nil when the file cannot be removed.
func (*Storage) DeleteFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}

	return nil
}

// Ensure reports whether the file is on disk, which is where the filesystem
// backend keeps every object.
//
// Parameters:
//   - ctx: Request scope, which the filesystem backend ignores.
//   - path: The file to check.
//
// Returns:
//   - err: Non-nil, wrapping [fs.ErrNotExist] for a missing file or a
//     directory, when the file cannot be served.
func (*Storage) Ensure(_ context.Context, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("ensure file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("ensure file: %s is a directory: %w", path, fs.ErrNotExist)
	}

	return nil
}

// Exists reports whether a regular file exists.
//
// Parameters:
//   - ctx: Request scope, which the filesystem backend ignores.
//   - path: The file to stat.
//
// Returns:
//   - exists: True when the path names an existing file, not a directory.
func (*Storage) Exists(_ context.Context, path string) bool {
	info, err := os.Stat(path)

	return err == nil && !info.IsDir()
}

// Put is a no-op on the filesystem backend because objects already live on disk.
//
// Parameters:
//   - ctx: Request scope, which the filesystem backend ignores.
//   - path: Scratch path, which the filesystem backend ignores.
//
// Returns:
//   - err: Always nil.
func (*Storage) Put(_ context.Context, _ string) error {
	return nil
}

// WriteThumbnail stores thumbnail bytes under the given id.
//
// Parameters:
//   - id: Identifier the thumbnail is filed under.
//   - data: The encoded thumbnail bytes.
//
// Returns:
//   - err: Non-nil when the file cannot be written.
func (store *Storage) WriteThumbnail(id string, data []byte) error {
	// Written beside the cache entry and renamed into place, so a concurrent
	// request never serves half a thumbnail.
	err := writeObjectFile(store.ThumbnailPath(id), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}

	return nil
}
