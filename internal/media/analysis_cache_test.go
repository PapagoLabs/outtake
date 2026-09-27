// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/media/crop"
)

// cropdetectStubLog is ffmpeg stderr that cropdetect produces for a 3840x2160
// source letterboxed to 3840x1608. It carries both the crop= values the parser
// reads and the Video: WxH line TrimsLog compares against, so the stubbed
// result is a rectangle that actually trims.
const cropdetectStubLog = "[Parsed_cropdetect_0 @ 0x1] crop=3840:1608:0:216\n" +
	"Stream #0:0: Video: hevc, yuv420p, 3840x2160 [SAR 1:1 DAR 16:9]\n"

// signalstatsStubLog is ffmpeg stderr carrying a YMAX reading the parser accepts.
const signalstatsStubLog = "lavfi.signalstats.YMAX=143.0\nlavfi.signalstats.YMAX=158.0\n"

// unusableBinary is a path that cannot be executed, used to prove a pass was
// answered from the cache rather than run.
const unusableBinary = "/nonexistent/ffmpeg"

// writeAnalysisStub returns a stand-in for ffmpeg that prints a fixed stderr
// payload, so the analysis passes can be exercised without a real ffmpeg.
//
// Parameters:
//   - t: Test context.
//   - stderr: Text the stub writes to standard error.
//
// Returns:
//   - path: Path of the stub to use as the ffmpeg binary.
func writeAnalysisStub(t *testing.T, stderr string) string {
	t.Helper()

	return stubScript(t, plainStubScript(stderr))
}

// writeAnalysisFixture creates a source file and returns its cache identity.
//
// Parameters:
//   - t: Test context.
//
// Returns:
//   - path: Path of the created file.
//   - identity: The cache identity for that file.
func writeAnalysisFixture(t *testing.T) (string, probeIdentity) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	identity, ok := probeKeyFor(path)
	require.True(t, ok)

	return path, identity
}

// cachedUnder reports whether an analysis key holds an entry.
func cachedUnder(key analysisKey) bool {
	analysisResults.mu.Lock()
	defer analysisResults.mu.Unlock()

	_, ok := analysisResults.entries[key]

	return ok
}

// TestDetectCropCachesByFileAndWindow covers the caching rule for cropdetect:
// the key is the file, the seek, and the window DetectArgs clamps to.
func TestDetectCropCachesByFileAndWindow(t *testing.T) {
	t.Parallel()

	path, identity := writeAnalysisFixture(t)
	execFFmpeg := NewExecFFmpeg(writeAnalysisStub(t, cropdetectStubLog), "unused")

	first, err := execFFmpeg.DetectCrop(t.Context(), path, 10, 30)
	require.NoError(t, err)
	require.Equal(t, CropRect{Width: 3840, Height: 1608, X: 0, Y: 216}, first,
		"the stubbed cropdetect log must be parsed into a trimming rectangle")

	key := analysisKeyFor(identity, analysisKindCrop, 10, crop.SampleDuration(30))
	assert.True(t, cachedUnder(key), "a successful pass must be cached")

	// A runner that cannot execute proves the second pass was cached rather than
	// re-run.
	blind := NewExecFFmpeg(unusableBinary, "unused")

	second, err := blind.DetectCrop(t.Context(), path, 10, 30)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

// TestDetectCropKeyTracksSeekAndWindow pins which requests share an entry.
//
// Both arguments matter: a different seek samples different frames, and a window
// below the cap is a genuinely different command.
func TestDetectCropKeyTracksSeekAndWindow(t *testing.T) {
	t.Parallel()

	path, _ := writeAnalysisFixture(t)
	execFFmpeg := NewExecFFmpeg(writeAnalysisStub(t, cropdetectStubLog), "unused")

	_, err := execFFmpeg.DetectCrop(t.Context(), path, 10, 30)
	require.NoError(t, err)

	blind := NewExecFFmpeg(unusableBinary, "unused")

	_, err = blind.DetectCrop(t.Context(), path, 12, 30)
	require.Error(t, err, "a different seek must not be served from the cache")

	_, err = blind.DetectCrop(t.Context(), path, 10, 1.5)
	require.Error(t, err, "a shorter sampling window must not be served from the cache")
}

// TestDetectCropSharesEntryAcrossClampedWindows is the saving this branch
// exists for: once the window is clamped, a longer clip samples the same frames
// and must reuse the entry.
func TestDetectCropSharesEntryAcrossClampedWindows(t *testing.T) {
	t.Parallel()

	path, _ := writeAnalysisFixture(t)
	execFFmpeg := NewExecFFmpeg(writeAnalysisStub(t, cropdetectStubLog), "unused")

	first, err := execFFmpeg.DetectCrop(t.Context(), path, 10, 3)
	require.NoError(t, err)

	blind := NewExecFFmpeg(unusableBinary, "unused")

	// 3 s and 600 s both clamp to the 3 s cap, so this is a cache hit.
	second, err := blind.DetectCrop(t.Context(), path, 10, 600)
	require.NoError(t, err, "a longer clip must reuse the clamped entry")
	assert.Equal(t, first, second)
}

// TestDetectCropMissesReplacedFile guards the identity check: a file replaced at
// the same path with matching metadata must not return the old rectangle.
func TestDetectCropMissesReplacedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(path, stamp, stamp))

	execFFmpeg := NewExecFFmpeg(writeAnalysisStub(t, cropdetectStubLog), "unused")

	_, err := execFFmpeg.DetectCrop(t.Context(), path, 10, 30)
	require.NoError(t, err)

	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, stamp, stamp))
	require.NoError(t, os.Rename(replacement, path))

	blind := NewExecFFmpeg(unusableBinary, "unused")

	_, err = blind.DetectCrop(t.Context(), path, 10, 30)
	require.Error(t, err, "a replaced file must miss the cache")
}

// TestSignalstatsCachesPeak covers the same rule for the PQ luma peak, over a
// float result.
func TestSignalstatsCachesPeak(t *testing.T) {
	t.Parallel()

	path, identity := writeAnalysisFixture(t)
	execFFmpeg := NewExecFFmpeg(writeAnalysisStub(t, signalstatsStubLog), "unused")

	first, ok := execFFmpeg.signalstatsYMax(t.Context(), path, 10, 30)
	require.True(t, ok, "the stubbed signalstats output must parse")
	assert.InDelta(t, 158.0, first, 0.001)

	key := analysisKeyFor(identity, analysisKindPeak, 10, peakSampleSeconds(30))
	assert.True(t, cachedUnder(key), "a successful sample must be cached")

	blind := NewExecFFmpeg(unusableBinary, "unused")

	second, ok := blind.signalstatsYMax(t.Context(), path, 10, 30)
	require.True(t, ok)
	assert.InDelta(t, first, second, 0.0005)

	// The 8 s cap means a longer clip reuses the entry.
	third, ok := blind.signalstatsYMax(t.Context(), path, 10, 600)
	require.True(t, ok, "a longer clip must reuse the clamped entry")
	assert.InDelta(t, first, third, 0.0005)
}

// TestSignalstatsDoesNotCacheFailure keeps a transient failure retryable.
//
// A pass that produced no luma is not the same as a source with no highlights,
// so caching a miss would pin a timeout in place for the process lifetime.
func TestSignalstatsDoesNotCacheFailure(t *testing.T) {
	t.Parallel()

	path, identity := writeAnalysisFixture(t)
	execFFmpeg := NewExecFFmpeg(writeAnalysisStub(t, "no signalstats output here"), "unused")

	_, ok := execFFmpeg.signalstatsYMax(t.Context(), path, 10, 30)
	require.False(t, ok)

	key := analysisKeyFor(identity, analysisKindPeak, 10, peakSampleSeconds(30))
	assert.False(t, cachedUnder(key), "a failed sample must not be cached")
}

// TestAnalysisWindowKeyRoundsLikeFFmpeg pins the quantisation to the resolution
// formatDuration renders, so two values producing an identical argv share one
// entry instead of occupying two.
func TestAnalysisWindowKeyRoundsLikeFFmpeg(t *testing.T) {
	t.Parallel()

	assert.Equal(t, analysisWindowKey(10), analysisWindowKey(10.0001),
		"a sub-millisecond difference must not create a second entry")
	assert.NotEqual(t, analysisWindowKey(10), analysisWindowKey(10.002))
	assert.Equal(t, int64(12_345), analysisWindowKey(12.345))
	assert.Equal(t, analysisWindowKey(0), analysisWindowKey(0.0001))
}

// writeAnalysisSwapStub returns a stand-in for ffmpeg that replaces the file
// under probe before answering, so a file changing underneath the pass can be
// reproduced without a real ffmpeg.
//
// The replacement is a sibling named swapReplacementName, which the stub resolves
// from the path it was asked about. That keeps the script identical between tests
// so it can be written once, up front.
//
// Parameters:
//   - t: Test context.
//   - stderr: Text the stub writes to standard error.
//
// Returns:
//   - path: Path of the stub to use as the ffmpeg binary.
func writeAnalysisSwapStub(t *testing.T, stderr string) string {
	t.Helper()

	return stubScript(t, swapStubScript(stderr))
}

// TestDetectCropSkipsCachingWhenFileChangesDuringProbe covers the re-check
// before a cropdetect result is stored.
//
// The stub swaps the file between the identity being taken and the result being
// stored. The replacement carries the same size and modification time, so the
// metadata key is unchanged and the file identity is the only thing that can tell
// the two apart. Nothing may be cached, because the identity recorded before the
// pass no longer describes the file at that path.
func TestDetectCropSkipsCachingWhenFileChangesDuringProbe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(target, []byte("original-content"), 0o600))

	before, ok := probeKeyFor(target)
	require.True(t, ok)

	// A different file of the same length and modification time.
	replacement := filepath.Join(dir, swapReplacementName)
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, before.key.mtime, before.key.mtime))

	execFFmpeg := NewExecFFmpeg(writeAnalysisSwapStub(t, cropdetectStubLog), "unused")

	rect, err := execFFmpeg.DetectCrop(t.Context(), target, 10, 30)
	require.NoError(t, err, "the stub answers regardless of what the file contains")
	require.NotZero(t, rect.Height, "the crop log must still be parsed")

	after, ok := probeKeyFor(target)
	require.True(t, ok)
	assert.Equal(
		t,
		before.key,
		after.key,
		"the metadata key must be unchanged, or this proves nothing",
	)
	assert.False(
		t,
		os.SameFile(before.file, after.file),
		"the file at target must now be a different file",
	)

	assert.False(
		t,
		cachedUnder(analysisKeyFor(before, analysisKindCrop, 10, crop.SampleDuration(30))),
		"a file that changed mid-pass must not be cached under the identity taken before it",
	)
}

// TestSignalstatsSkipsCachingWhenFileChangesDuringProbe covers the same re-check
// on the luma path.
func TestSignalstatsSkipsCachingWhenFileChangesDuringProbe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(target, []byte("original-content"), 0o600))

	before, ok := probeKeyFor(target)
	require.True(t, ok)

	replacement := filepath.Join(dir, swapReplacementName)
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, before.key.mtime, before.key.mtime))

	execFFmpeg := NewExecFFmpeg(writeAnalysisSwapStub(t, signalstatsStubLog), "unused")

	peak, ok := execFFmpeg.signalstatsYMax(t.Context(), target, 10, 30)
	require.True(t, ok, "the stub answers regardless of what the file contains")
	assert.InDelta(t, 158.0, peak, 0.001)

	assert.False(
		t,
		cachedUnder(analysisKeyFor(before, analysisKindPeak, 10, peakSampleSeconds(30))),
		"a file that changed mid-pass must not be cached under the identity taken before it",
	)
}

// TestPeakSampleSecondsPinsTheWindow covers the clamp that both the cache key
// and the ffmpeg argv derive from.
func TestPeakSampleSecondsPinsTheWindow(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, float64(webSafePeakSecs), peakSampleSeconds(600), 0.001)
	assert.InDelta(t, float64(webSafePeakSecs), peakSampleSeconds(0), 0.001)
	assert.InDelta(t, 3.5, peakSampleSeconds(3.5), 0.001)
}

// TestAnalysisCacheEvictsAtCapacity proves the bound is enforced, by filling the
// cache and then overflowing it.
func TestAnalysisCacheEvictsAtCapacity(t *testing.T) {
	t.Parallel()

	cache := &analysisCache{}
	dir := t.TempDir()
	keys := make([]analysisKey, 0, analysisCacheMaxEntries+1)

	for i := range analysisCacheMaxEntries + 1 {
		path := filepath.Join(dir, "clip"+strconv.Itoa(i)+".mkv")
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

		identity, ok := probeKeyFor(path)
		require.True(t, ok)

		key := analysisKeyFor(identity, analysisKindPeak, 0, 0)

		keys = append(keys, key)
		cache.put(key, analysisValue{peak: float64(i)}, identity.file)
	}

	overflowed := keys[len(keys)-1]
	earliest := keys[0]

	got, ok := cache.get(overflowed, nil)
	require.True(t, ok, "the entry written past the limit must be kept")
	assert.InDelta(t, float64(len(keys)-1), got.peak, 0.001)

	_, ok = cache.get(earliest, nil)
	assert.False(t, ok, "overflowing the cache must drop the earlier entries")

	cache.mu.Lock()

	size := len(cache.entries)
	cache.mu.Unlock()

	assert.LessOrEqual(t, size, analysisCacheMaxEntries)
}
