// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"crypto/sha256"
	"encoding/hex"
)

// CacheControl tells the browser to reuse a thumbnail for a week. It is
// private, because a thumbnail is only served to a signed-in session and a
// shared cache must not hand it to anyone else.
const CacheControl = "private, max-age=604800, immutable"

// CacheID hashes a server and a Plex thumb path into a cache filename. Two
// servers can use the same thumb path for different images, so the server is
// part of the key.
//
// Parameters:
//   - server: Identity of the Plex server the thumb comes from.
//   - thumbPath: Plex thumb path.
//
// Returns:
//   - id: Hex-encoded SHA-256 digest used as the cache filename.
func CacheID(server, thumbPath string) string {
	sum := sha256.Sum256([]byte(server + "\x00" + thumbPath))

	return hex.EncodeToString(sum[:])
}
