// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob

import (
	"context"
	"errors"
)

// Blob moves media between local disk and its configured home.
type Blob interface {
	// DeleteFile deletes the object identified by path.
	DeleteFile(path string) error
	// FileExists reports whether the object identified by path exists.
	FileExists(path string) bool
	// Get downloads the object to the local path for ffmpeg or HTTP serving.
	Get(ctx context.Context, path string) error
	// Put uploads the local path after ffmpeg writes it.
	Put(ctx context.Context, path string) error
	// WriteThumbnail stores thumbnail bytes under the given id.
	WriteThumbnail(id string, data []byte) error
}

// The backend names accepted by the storage-backend setting.
const (
	// BackendFilesystem is the local filesystem backend.
	BackendFilesystem = "filesystem"
	// BackendS3 is the S3-compatible object backend.
	BackendS3 = "s3"
)

// ErrUnknownStorageBackend is returned when the configured backend is unknown.
var ErrUnknownStorageBackend = errors.New("unknown storage backend")

// Both backends satisfy the Blob port.
var (
	_ Blob = (*Storage)(nil)
	_ Blob = (*S3)(nil)
)
