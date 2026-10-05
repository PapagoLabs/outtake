// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// cacheKey identifies a probed file by identity on disk.
type cacheKey struct {
	path  string
	mtime time.Time
	size  int64
}

// Key is a probed file's identity on disk.
type Key struct {
	key  cacheKey
	file os.FileInfo
}

// cacheEntry is one cached probe result and the file it describes.
type cacheEntry struct {
	info Info
	file os.FileInfo
}

// probeCache stores probe results by file identity.
type probeCache struct {
	mu      sync.Mutex
	entries map[cacheKey]cacheEntry
}

// cacheMaxEntries bounds the probe cache.
const cacheMaxEntries = 256

// probeResults is the process wide probe cache.
var probeResults probeCache

// KeyFor builds the cache identity for a path, reporting whether the file can be
// identified on disk.
//
// Parameters:
//   - path: Media file path.
//
// Returns:
//   - key: The cache identity for the file.
//   - ok: False when the file cannot be stat'd.
func KeyFor(path string) (Key, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return Key{}, false
	}

	return Key{
		key:  cacheKey{path: filepath.Clean(path), mtime: info.ModTime(), size: info.Size()},
		file: info,
	}, true
}

// identityUnchanged reports whether a file still carries the identity that was
// recorded before it was probed.
//
// Parameters:
//   - path: Media file path.
//   - before: Identity captured before probing.
//
// Returns:
//   - same: True when path still resolves to the same unchanged file.
func identityUnchanged(path string, before Key) bool {
	after, ok := KeyFor(path)
	if !ok {
		return false
	}

	if before.file == nil || after.file == nil {
		return false
	}

	return before.key == after.key && os.SameFile(before.file, after.file)
}

// get returns a cached probe result.
//
// Parameters:
//   - identity: Cache identity for the file.
//
// Returns:
//   - info: The cached result, with its audio tracks copied.
//   - ok: True when the file had been probed and still is that file.
func (cache *probeCache) get(identity Key) (Info, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	entry, ok := cache.entries[identity.key]
	if !ok {
		return Info{}, false
	}

	if identity.file != nil && entry.file != nil && !os.SameFile(identity.file, entry.file) {
		return Info{}, false
	}

	return copyInfo(entry.info), true
}

// put stores a probe result.
//
// Parameters:
//   - identity: Cache identity for the file.
//   - info: Probe result to store.
func (cache *probeCache) put(identity Key, info Info) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.entries == nil {
		cache.entries = make(map[cacheKey]cacheEntry)
	}

	if len(cache.entries) >= cacheMaxEntries {
		clear(cache.entries)
	}

	cache.entries[identity.key] = cacheEntry{info: copyInfo(info), file: identity.file}
}

// copyInfo returns a copy of info whose audio track slice is independent.
//
// Parameters:
//   - info: Result to copy.
//
// Returns:
//   - copy: An independent copy of info.
func copyInfo(info Info) Info {
	info.AudioTracks = slices.Clone(info.AudioTracks)

	return info
}
