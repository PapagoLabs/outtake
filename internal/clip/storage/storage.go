// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package storage provides blob storage for media files.
package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Storage provides filesystem storage.
type Storage struct {
	basePath string
}

const (
	// dirPermissions is the default directory permissions.
	dirPermissions = 0o755

	// filePermissions is the default file permissions.
	filePermissions = 0o644
)

// NewStorage creates a new storage instance.
//
// Parameters:
//   - basePath: Base path.
//
// Returns:
//   - storage: A new storage instance.
//   - err: The error, if any.
func NewStorage(basePath string) (*Storage, error) {
	err := os.MkdirAll(basePath, dirPermissions)
	if err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}

	subdirs := []string{"clips", "gifs", "previews", "screenshots", "thumbnails"}
	for _, sub := range subdirs {
		dir := filepath.Join(basePath, sub)
		err := os.MkdirAll(dir, dirPermissions)
		if err != nil {
			return nil, fmt.Errorf("create %s dir: %w", sub, err)
		}
	}

	return &Storage{basePath: basePath}, nil
}

// BasePath returns the base storage path.
//
// Returns:
//   - path: The base storage path.
func (s *Storage) BasePath() string {
	return s.basePath
}

// ClipPath returns the path for a clip file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The path for a clip file.
func (s *Storage) ClipPath(id string) string {
	return filepath.Join(s.ClipsDir(), id+".mp4")
}

// ClipsDir returns the clips directory.
//
// Returns:
//   - value: The clips directory.
func (s *Storage) ClipsDir() string {
	return filepath.Join(s.basePath, "clips")
}

// DeleteFile deletes a file.
//
// Parameters:
//   - path: Filesystem path.
//
// Returns:
//   - err: The error, if any.
func (*Storage) DeleteFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}

	return nil
}

// FileExists reports whether a file exists.
//
// Parameters:
//   - path: Filesystem path.
//
// Returns:
//   - ok: True when a file exists.
func (*Storage) FileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// Get is a no-op on the filesystem backend because objects already live on
// disk.
//
// Returns:
//   - err: The error, if any.
func (*Storage) Get(_ context.Context, _ string) error {
	return nil
}

// GifPath returns the path for a GIF file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The path for a GIF file.
func (s *Storage) GifPath(id string) string {
	return filepath.Join(s.GifsDir(), id+".gif")
}

// GifsDir returns the GIFs directory.
//
// Returns:
//   - value: The GIFs directory.
func (s *Storage) GifsDir() string {
	return filepath.Join(s.basePath, "gifs")
}

// PreviewPath returns the path for a temporary preview file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The path for a temporary preview file.
func (s *Storage) PreviewPath(id string) string {
	return filepath.Join(s.PreviewsDir(), id+".mp4")
}

// PreviewsDir returns the previews directory.
//
// Returns:
//   - value: The previews directory.
func (s *Storage) PreviewsDir() string {
	return filepath.Join(s.basePath, "previews")
}

// Put is a no-op on the filesystem backend because objects already live on
// disk.
//
// Returns:
//   - err: The error, if any.
func (*Storage) Put(_ context.Context, _ string) error {
	return nil
}

// ScreenshotPath returns the path for a screenshot file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The path for a screenshot file.
func (s *Storage) ScreenshotPath(id string) string {
	return filepath.Join(s.ScreenshotsDir(), id+".jpg")
}

// ScreenshotsDir returns the screenshots directory.
//
// Returns:
//   - value: The screenshots directory.
func (s *Storage) ScreenshotsDir() string {
	return filepath.Join(s.basePath, "screenshots")
}

// ThumbnailPath returns the path for a thumbnail file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The path for a thumbnail file.
func (s *Storage) ThumbnailPath(id string) string {
	return filepath.Join(s.ThumbnailsDir(), id+".jpg")
}

// ThumbnailsDir returns the thumbnails directory.
//
// Returns:
//   - value: The thumbnails directory.
func (s *Storage) ThumbnailsDir() string {
	return filepath.Join(s.basePath, "thumbnails")
}

// WriteThumbnail stores thumbnail bytes under the given id.
//
// Parameters:
//   - id: Identifier.
//   - data: Data.
//
// Returns:
//   - err: The error, if any.
func (s *Storage) WriteThumbnail(id string, data []byte) error {
	err := os.WriteFile(s.ThumbnailPath(id), data, filePermissions)
	if err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}

	return nil
}
