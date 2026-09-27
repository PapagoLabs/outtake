// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"math"
	"os"
	"sync"
)

// analysisKind selects which field of a cached result is meaningful.
type analysisKind uint8

// analysisKey identifies one analysis of a file at a seek position and window.
//
// The window is the value actually passed to ffmpeg, already clamped by the
// caller. Two requests whose windows clamp to the same value run an identical
// command and share an entry, which is where the saving comes from: the crop and
// luma passes both cap how much of the source they sample, so a long clip and a
// shorter one produce the same work.
type analysisKey struct {
	kind   analysisKind
	file   probeCacheKey
	start  int64
	window int64
}

// analysisValue is a cached result. Only the field named by the key's kind is
// meaningful.
type analysisValue struct {
	rect CropRect
	peak float64
}

// analysisEntry is a cached result and the file it was computed from.
type analysisEntry struct {
	value analysisValue
	owner os.FileInfo
}

// analysisCache stores analysis results by file identity, seek, and window.
//
// It is package level for the same reason as the probe cache: callers construct a
// fresh ExecFFmpeg per request, so a field on the runner would never be shared.
type analysisCache struct {
	mu      sync.Mutex
	entries map[analysisKey]analysisEntry
}

const (
	// AnalysisKindCrop caches a cropdetect rectangle.
	analysisKindCrop analysisKind = iota
	// AnalysisKindPeak caches the PQ luma peak.
	analysisKindPeak
)

const (
	// AnalysisCacheMaxEntries bounds the cache.
	//
	// A cap is needed because it is keyed by file, so a long browsing session
	// would otherwise retain an entry for every file analyzed. Overflow clears
	// the cache, which is O(1) and cannot serve a stale value.
	analysisCacheMaxEntries = 256

	// AnalysisMillis is the resolution formatDuration renders, and therefore the
	// resolution a seek or window is compared at.
	analysisMillis = 1000
)

// analysisResults caches crop rectangles and luma peaks by file identity.
var analysisResults analysisCache

// analysisWindowKey quantises a seconds value to the resolution ffmpeg is given.
//
// A cache key has to match the command that produced the result, so this uses
// the same millisecond rounding that formatDuration applies. Without it,
// 1.0001 and 1.0004 would occupy separate entries despite an identical argv.
//
// Parameters:
//   - seconds: Offset or window in seconds.
//
// Returns:
//   - millis: The value in whole milliseconds.
func analysisWindowKey(seconds float64) int64 {
	return int64(math.Round(seconds * analysisMillis))
}

// lookupAnalysis returns a cached result for one analysis.
//
// An identity with no file could not be stat'd, so it has no stable key and
// never consults or fills the cache.
//
// Parameters:
//   - identity: Cache identity of the source file.
//   - kind: Which analysis to look up.
//   - start: Seek offset in seconds.
//   - window: Sampling window in seconds.
//
// Returns:
//   - value: The cached result.
//   - found: True when the file had been analyzed and still is that file.
func lookupAnalysis(
	identity probeIdentity,
	kind analysisKind,
	start, window float64,
) (analysisValue, bool) {
	if identity.file == nil {
		return analysisValue{}, false
	}

	return analysisResults.get(analysisKeyFor(identity, kind, start, window), identity.file)
}

// storeAnalysis caches one result, but only if the source file is unchanged
// since the analysis read its identity.
//
// A file that is replaced or rewritten while ffmpeg is sampling produces a
// result that describes neither the file the identity names nor the file now on
// disk, so it is returned to the caller and never cached. The read side re-checks
// identity too, but catching it here keeps a dead entry out of the cache.
//
// Parameters:
//   - path: Cleaned source path, re-read to confirm the identity.
//   - identity: Cache identity of the source file.
//   - kind: Which analysis is being cached.
//   - start: Seek offset in seconds.
//   - window: Sampling window in seconds.
//   - value: Result to store.
func storeAnalysis(
	path string,
	identity probeIdentity,
	kind analysisKind,
	start, window float64,
	value analysisValue,
) {
	if identity.file == nil || !probeIdentityUnchanged(path, identity) {
		return
	}

	analysisResults.put(analysisKeyFor(identity, kind, start, window), value, identity.file)
}

// analysisKeyFor builds the cache key for one analysis.
//
// Parameters:
//   - identity: Cache identity of the source file.
//   - kind: Which analysis is being cached.
//   - start: Seek offset in seconds.
//   - window: Sampling window in seconds, already clamped by the caller.
//
// Returns:
//   - key: The cache key for the analysis.
func analysisKeyFor(identity probeIdentity, kind analysisKind, start, window float64) analysisKey {
	return analysisKey{
		kind:   kind,
		file:   identity.key,
		start:  analysisWindowKey(start),
		window: analysisWindowKey(window),
	}
}

// get returns a cached result.
//
// A key hit whose file identity no longer matches is treated as a miss, so a file
// replaced at the same path with the same size and modification time cannot
// return the previous file's analysis.
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
// The cache is dropped wholesale once it is full, matching the probe cache.
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
