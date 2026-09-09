// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/PapagoLabs/outtake/internal/config"
)

// Blob is the media object store used by the application.
type Blob interface {
	// ClipPath returns the local path for a clip file.
	ClipPath(id string) string
	// DeleteFile deletes the object identified by path.
	DeleteFile(path string) error
	// FileExists reports whether the object identified by path exists.
	FileExists(path string) bool
	// Get downloads the object to the local path for ffmpeg or HTTP serving.
	Get(ctx context.Context, path string) error
	// GifPath returns the local path for a GIF file.
	GifPath(id string) string
	// PreviewPath returns the local path for a preview file.
	PreviewPath(id string) string
	// Put uploads the local path after ffmpeg writes it.
	Put(ctx context.Context, path string) error
	// ScreenshotPath returns the local path for a screenshot file.
	ScreenshotPath(id string) string
	// ThumbnailPath returns the local path for a thumbnail file.
	ThumbnailPath(id string) string
	// WriteThumbnail stores thumbnail bytes under the given id.
	WriteThumbnail(id string, data []byte) error
}

const (
	// storageBackendFilesystem is the local filesystem backend.
	storageBackendFilesystem = "filesystem"
	// storageBackendS3 is the S3-compatible object backend.
	storageBackendS3 = "s3"
	// defaultS3Region is the region used when none is configured.
	defaultS3Region = "us-east-1"
)

var (
	// errUnknownStorageBackend is returned when StorageBackend is not recognized.
	errUnknownStorageBackend = errors.New("unknown storage backend")
	// errS3BucketRequired is returned when S3 is selected without a bucket.
	errS3BucketRequired = errors.New("s3-bucket is required")
	// errS3EndpointRequired is returned when S3 is selected without an endpoint.
	errS3EndpointRequired = errors.New("s3-endpoint is required")
)

var _ Blob = (*Storage)(nil)

// NewFromConfig constructs the storage backend selected by configuration.
//
// Parameters:
//   - cfg: Application configuration.
//
// Returns:
//   - blob: The storage backend selected by configuration.
//   - err: The error, if any.
func NewFromConfig( //nolint:ireturn // Factory selects the configured backend.
	cfg *config.Config,
) (Blob, error) {
	backend := strings.ToLower(strings.TrimSpace(cfg.StorageBackend))
	switch backend {
	case "", storageBackendFilesystem:
		store, err := NewStorage(cfg.StoragePath)
		if err != nil {
			return nil, fmt.Errorf("filesystem storage: %w", err)
		}

		return store, nil
	case storageBackendS3:
		store, err := newS3FromConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("s3 storage: %w", err)
		}

		return store, nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnknownStorageBackend, cfg.StorageBackend)
	}
}
