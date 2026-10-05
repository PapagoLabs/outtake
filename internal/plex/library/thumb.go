// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"crypto/sha256"
	"encoding/hex"
)

// CacheControl tells browsers to reuse cached thumbnails for a week.
const CacheControl = "public, max-age=604800, immutable"

// CacheID hashes a Plex thumb path into a cache filename.
//
// Parameters:
//   - thumbPath: Plex thumb path.
//
// Returns:
//   - id: Hex-encoded SHA-256 digest used as the cache filename.
func CacheID(thumbPath string) string {
	sum := sha256.Sum256([]byte(thumbPath))

	return hex.EncodeToString(sum[:])
}
