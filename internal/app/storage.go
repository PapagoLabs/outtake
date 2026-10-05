// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"fmt"
	"strings"

	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

// blobStore is the storage wiring the application hands to its services.
type blobStore struct {
	// blob moves media between local disk and its configured home.
	blob blob.Blob
	// paths builds the local paths blob moves.
	paths blob.Paths
}

// initStorage selects the storage backend the configuration asks for.
//
// Parameters:
//   - cfg: App config. StorageBackend names the backend and StoragePath is the
//     local directory every backend keeps its scratch copy in.
//
// Returns:
//   - store: The selected transfer port and its local layout.
//   - err: Non-nil when the backend is unknown or cannot be constructed.
func initStorage(cfg *config.Config) (blobStore, error) {
	paths := blob.NewPaths(cfg.StoragePath)

	backend := strings.ToLower(strings.TrimSpace(cfg.StorageBackend))

	switch backend {
	case blob.BackendS3:
		store, err := blob.NewS3(cfg, paths)
		if err != nil {
			return blobStore{}, fmt.Errorf("s3 storage: %w", err)
		}

		return blobStore{blob: store, paths: paths}, nil
	case "", blob.BackendFilesystem:
		store, err := blob.NewStorage(paths)
		if err != nil {
			return blobStore{}, fmt.Errorf("filesystem storage: %w", err)
		}

		return blobStore{blob: store, paths: paths}, nil
	default:
		return blobStore{}, fmt.Errorf(
			"%w: %s", blob.ErrUnknownStorageBackend, cfg.StorageBackend,
		)
	}
}
