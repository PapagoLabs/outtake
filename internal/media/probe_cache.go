// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// probeCacheKey identifies a probed file by identity on disk.
//
// Modification time and size are part of the key rather than being validated on
// read, so a re-transcoded file simply misses and its stale entry becomes
// garbage. That avoids needing a purge path, and means the key stays correct if
// previews later become content addressed.
type probeCacheKey struct {
	path  string
	mtime time.Time
	size  int64
}

// probeIdentity is a cache key together with the file identity it was taken
// from.
type probeIdentity struct {
	key  probeCacheKey
	file os.FileInfo
}

// probeCacheEntry is one cached probe result and the file it describes.
type probeCacheEntry struct {
	info MediaInfo
	file os.FileInfo
}

// probeCache stores probe results by file identity.
//
// It is package level because callers construct a fresh ExecFFmpeg per request,
// so a field on the runner would never be shared between them.
type probeCache struct {
	mu      sync.Mutex
	entries map[probeCacheKey]probeCacheEntry
}

// probeCacheMaxEntries bounds the probe cache.
//
// A cap is needed because the cache is keyed by media file path, so a long
// browsing session would otherwise retain an entry for every file ever opened.
// Unlike the added-at index cache, which is keyed by a small set of library
// and sort combinations, this one grows with the library.
const probeCacheMaxEntries = 256

// probeResults is the process wide probe cache.
var probeResults probeCache

// probeKeyFor builds the cache identity for a path, reporting whether the file
// can be identified on disk.
//
// A file that cannot be stat'd has no stable identity, so the caller must bypass
// the cache rather than risk a key that would collide across replacements.
//
// Path, size, and modification time alone are not enough. A file retargeted in
// place, or replaced by a rename that preserves its metadata, keeps all three
// while pointing at entirely different content, so the file identity captured
// here is checked again on every read. That catches replacement and retargeting.
// It does not catch an in-place rewrite that restores both the original size and
// the original modification time, which would need the content hashed to
// distinguish and is not worth reading a multi-gigabyte file for.
//
// Parameters:
//   - path: Media file path.
//
// Returns:
//   - identity: The cache identity for the file.
//   - ok: False when the file cannot be stat'd.
func probeKeyFor(path string) (probeIdentity, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return probeIdentity{}, false
	}

	return probeIdentity{
		key:  probeCacheKey{path: filepath.Clean(path), mtime: info.ModTime(), size: info.Size()},
		file: info,
	}, true
}

// probeIdentityUnchanged reports whether a file still carries the identity that
// was recorded before it was probed.
//
// Probing a large source takes long enough for the file to be replaced or
// rewritten underneath the probe. Storing the result under an identity the file
// no longer has would attribute one version's metadata to another, so a file
// that moved on mid-probe is reported to the caller but not cached.
//
// Parameters:
//   - path: Media file path.
//   - before: Identity captured before probing.
//
// Returns:
//   - same: True when path still resolves to the same unchanged file.
func probeIdentityUnchanged(path string, before probeIdentity) bool {
	after, ok := probeKeyFor(path)
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
// A key hit whose file identity no longer matches is treated as a miss, so a
// file replaced at the same path with the same size and modification time
// cannot return the previous file's metadata.
//
// Parameters:
//   - identity: Cache identity for the file.
//
// Returns:
//   - info: The cached result, with its audio tracks copied.
//   - ok: True when the file had been probed and still is that file.
func (cache *probeCache) get(identity probeIdentity) (MediaInfo, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	entry, ok := cache.entries[identity.key]
	if !ok {
		return MediaInfo{}, false
	}

	if identity.file != nil && entry.file != nil && !os.SameFile(identity.file, entry.file) {
		return MediaInfo{}, false
	}

	return copyMediaInfo(entry.info), true
}

// put stores a probe result.
//
// The cache is dropped wholesale once it is full. That is O(1) and cannot serve
// a stale value, which matters more here than keeping the entries.
//
// Parameters:
//   - identity: Cache identity for the file.
//   - info: Probe result to store.
func (cache *probeCache) put(identity probeIdentity, info MediaInfo) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.entries == nil {
		cache.entries = make(map[probeCacheKey]probeCacheEntry)
	}

	if len(cache.entries) >= probeCacheMaxEntries {
		clear(cache.entries)
	}

	cache.entries[identity.key] = probeCacheEntry{info: copyMediaInfo(info), file: identity.file}
}

// copyMediaInfo returns a copy of info whose audio track slice is independent.
//
// MediaInfo is shared between callers once cached, and AudioTracks is a slice,
// so handing out the cached one would let a caller mutate another's view.
//
// Parameters:
//   - info: Result to copy.
//
// Returns:
//   - copy: An independent copy of info.
func copyMediaInfo(info MediaInfo) MediaInfo {
	info.AudioTracks = slices.Clone(info.AudioTracks)

	return info
}
