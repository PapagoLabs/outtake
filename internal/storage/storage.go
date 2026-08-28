// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package storage provides filesystem storage for media files.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// Storage provides filesystem storage.
type Storage struct {
	basePath string
}

const (
	// DirPermissions is the default directory permissions.
	dirPermissions = 0o755

	// FilePermissions is the default file permissions.
	filePermissions = 0o644
)

// NewStorage creates a new storage instance.
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
func (s *Storage) BasePath() string {
	return s.basePath
}

// ClipPath returns the path for a clip file.
func (s *Storage) ClipPath(id string) string {
	return filepath.Join(s.ClipsDir(), id+".mp4")
}

// ClipsDir returns the clips directory.
func (s *Storage) ClipsDir() string {
	return filepath.Join(s.basePath, "clips")
}

// DeleteFile deletes a file.
func (*Storage) DeleteFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}

	return nil
}

// FileExists checks if a file exists.
func (*Storage) FileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// GifPath returns the path for a GIF file.
func (s *Storage) GifPath(id string) string {
	return filepath.Join(s.GifsDir(), id+".gif")
}

// GifsDir returns the GIFs directory.
func (s *Storage) GifsDir() string {
	return filepath.Join(s.basePath, "gifs")
}

// PreviewPath returns the path for a temporary preview file.
func (s *Storage) PreviewPath(id string) string {
	return filepath.Join(s.PreviewsDir(), id+".mp4")
}

// PreviewsDir returns the previews directory.
func (s *Storage) PreviewsDir() string {
	return filepath.Join(s.basePath, "previews")
}

// ScreenshotPath returns the path for a screenshot file.
func (s *Storage) ScreenshotPath(id string) string {
	return filepath.Join(s.ScreenshotsDir(), id+".jpg")
}

// ScreenshotsDir returns the screenshots directory.
func (s *Storage) ScreenshotsDir() string {
	return filepath.Join(s.basePath, "screenshots")
}

// ThumbnailPath returns the path for a thumbnail file.
func (s *Storage) ThumbnailPath(id string) string {
	return filepath.Join(s.ThumbnailsDir(), id+".jpg")
}

// ThumbnailsDir returns the thumbnails directory.
func (s *Storage) ThumbnailsDir() string {
	return filepath.Join(s.basePath, "thumbnails")
}

// WriteThumbnail stores thumbnail bytes under the given id.
func (s *Storage) WriteThumbnail(id string, data []byte) error {
	err := os.WriteFile(s.ThumbnailPath(id), data, filePermissions)
	if err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}

	return nil
}
