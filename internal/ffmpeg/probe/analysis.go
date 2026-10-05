// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"os"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
)

// kind selects which field of a cached analysis is meaningful.
type kind uint8

// analysisKey identifies one analysis of a file at a seek position and window.
type analysisKey struct {
	kind   kind
	file   cacheKey
	start  int64
	window int64
}

// analysisValue holds a crop rectangle or a luma peak, selected by kind.
type analysisValue struct {
	rect crop.CropRect
	peak float64
}

// analysisEntry is a cached result and the file it was computed from.
type analysisEntry struct {
	value analysisValue
	owner os.FileInfo
}

// analysisCache stores analysis results by file identity, seek, and window.
type analysisCache struct {
	mu      sync.Mutex
	entries map[analysisKey]analysisEntry
}

const (
	// kindCrop selects the cached cropdetect rectangle.
	kindCrop kind = iota
	// kindPeak selects the cached PQ luma peak.
	kindPeak
)

const (
	// analysisCacheMaxEntries caps how many analyses are held at once.
	analysisCacheMaxEntries = 256
)

// analysisResults caches crop rectangles and luma peaks by file identity.
var analysisResults analysisCache

// CachedCrop returns a cached crop rectangle for one analysis.
//
// Parameters:
//   - start: Seek offset.
//   - window: Sampling window, already clamped by the caller.
//
// Returns:
//   - rect: The cached rectangle.
//   - ok: True when the file had been analyzed and still is that file.
func (key Key) CachedCrop(start, window time.Duration) (crop.CropRect, bool) {
	value, found := lookupAnalysis(key, kindCrop, start, window)
	if !found {
		return crop.CropRect{}, false
	}

	return value.rect, true
}

// CachedPeak returns a cached PQ luma peak for one analysis.
//
// Parameters:
//   - start: Seek offset.
//   - window: Sampling window, already clamped by the caller.
//
// Returns:
//   - peak: The cached peak.
//   - ok: True when the file had been analyzed and still is that file.
func (key Key) CachedPeak(start, window time.Duration) (float64, bool) {
	value, found := lookupAnalysis(key, kindPeak, start, window)
	if !found {
		return 0, false
	}

	return value.peak, true
}

// StoreCrop caches a crop rectangle, but only if the source file is unchanged
// since the analysis read its identity.
//
// Parameters:
//   - path: Cleaned source path, re-read to confirm the identity.
//   - key: Cache identity of the source file.
//   - start: Seek offset.
//   - window: Sampling window.
//   - rect: Result to store.
func StoreCrop(path string, key Key, start, window time.Duration, rect crop.CropRect) {
	storeAnalysis(path, key, kindCrop, start, window, analysisValue{rect: rect})
}

// StorePeak caches a PQ luma peak under the same identity rules as StoreCrop.
//
// Parameters:
//   - path: Cleaned source path, re-read to confirm the identity.
//   - key: Cache identity of the source file.
//   - start: Seek offset.
//   - window: Sampling window.
//   - peak: Result to store.
func StorePeak(path string, key Key, start, window time.Duration, peak float64) {
	storeAnalysis(path, key, kindPeak, start, window, analysisValue{peak: peak})
}

// windowKey quantises a duration to the resolution ffmpeg is given.
//
// Parameters:
//   - duration: Offset or window.
//
// Returns:
//   - millis: The value in whole milliseconds.
func windowKey(duration time.Duration) int64 {
	return duration.Round(time.Millisecond).Milliseconds()
}

// lookupAnalysis returns a cached result for one analysis.
//
// Parameters:
//   - identity: Cache identity of the source file.
//   - kind: Which analysis to look up.
//   - start: Seek offset.
//   - window: Sampling window, already clamped by the caller.
//
// Returns:
//   - value: The cached result.
//   - found: True when the file had been analyzed and still is that file.
func lookupAnalysis(
	identity Key,
	kind kind,
	start, window time.Duration,
) (analysisValue, bool) {
	if identity.file == nil {
		return analysisValue{}, false
	}

	return analysisResults.get(analysisKeyFor(identity, kind, start, window), identity.file)
}

// storeAnalysis caches one result, but only if the source file is unchanged
// since the analysis read its identity.
//
// Parameters:
//   - path: Cleaned source path, re-read to confirm the identity.
//   - identity: Cache identity of the source file.
//   - kind: Which analysis is being cached.
//   - start: Seek offset.
//   - window: Sampling window, already clamped by the caller.
//   - value: Result to store.
func storeAnalysis(
	path string,
	identity Key,
	kind kind,
	start, window time.Duration,
	value analysisValue,
) {
	if identity.file == nil || !identityUnchanged(path, identity) {
		return
	}

	analysisResults.put(analysisKeyFor(identity, kind, start, window), value, identity.file)
}

// analysisKeyFor builds the cache key for one analysis.
//
// Parameters:
//   - identity: Cache identity of the source file.
//   - kind: Which analysis to look up.
//   - start: Seek offset.
//   - window: Sampling window, already clamped by the caller.
//
// Returns:
//   - key: The cache key for the analysis.
func analysisKeyFor(identity Key, kind kind, start, window time.Duration) analysisKey {
	return analysisKey{
		kind:   kind,
		file:   identity.key,
		start:  windowKey(start),
		window: windowKey(window),
	}
}

// get returns a cached result.
//
// Parameters:
//   - key: Cache key for the analysis.
//   - current: File identity read before the lookup.
//
// Returns:
//   - value: The cached result.
//   - ok: True when the file had been analyzed and still is that file.
func (cache *analysisCache) get(key analysisKey, current os.FileInfo) (analysisValue, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	entry, ok := cache.entries[key]
	if !ok {
		return analysisValue{}, false
	}

	if current != nil && entry.owner != nil && !os.SameFile(current, entry.owner) {
		return analysisValue{}, false
	}

	return entry.value, true
}

// put stores a result.
//
// Parameters:
//   - key: Cache key for the analysis.
//   - value: Result to store.
//   - owner: File identity the result was computed from.
func (cache *analysisCache) put(key analysisKey, value analysisValue, owner os.FileInfo) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.entries == nil {
		cache.entries = make(map[analysisKey]analysisEntry)
	}

	if len(cache.entries) >= analysisCacheMaxEntries {
		clear(cache.entries)
	}

	cache.entries[key] = analysisEntry{value: value, owner: owner}
}
