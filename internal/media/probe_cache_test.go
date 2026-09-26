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
)

// missingBinary is a path that cannot be executed, used to prove the cache is
// consulted without ffprobe ever running.
const missingBinary = "/nonexistent/ffprobe"

// writeProbeFixture creates a media file and returns its cache identity.
//
// Parameters:
//   - t: Test context.
//   - contents: Bytes to write.
//
// Returns:
//   - path: Path of the created file.
//   - identity: The cache identity for that file.
func writeProbeFixture(t *testing.T, contents string) (string, probeIdentity) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))

	identity, ok := probeKeyFor(path)
	require.True(t, ok)

	return path, identity
}

// TestProbeReturnsCachedResult proves the cache is wired into Probe rather than
// merely existing alongside it.
//
// The runner points at a binary that cannot be executed, so a result can only
// come back if Probe answered from the cache without shelling out.
func TestProbeReturnsCachedResult(t *testing.T) {
	t.Parallel()

	path, identity := writeProbeFixture(t, "seeded")

	want := MediaInfo{
		Duration:      42,
		Width:         3840,
		Height:        2160,
		VideoCodec:    "hevc",
		ColorTransfer: "smpte2084",
		AudioTracks:   []AudioTrack{{Index: 0, Codec: "eac3"}},
	}
	probeResults.put(identity, want)

	execFFmpeg := NewExecFFmpeg("/nonexistent/ffmpeg", missingBinary)

	got, err := execFFmpeg.Probe(t.Context(), path)
	require.NoError(t, err)
	assert.InDelta(t, want.Duration, got.Duration, 0.0005)
	assert.Equal(t, want.Width, got.Width)
	assert.Equal(t, want.ColorTransfer, got.ColorTransfer)
	assert.Equal(t, want.AudioTracks, got.AudioTracks)
}

// TestProbeKeyTracksFileIdentity covers what makes the key correct: a file that
// changes on disk has to miss, or a re-transcode would serve stale metadata.
func TestProbeKeyTracksFileIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))

	first, ok := probeKeyFor(path)
	require.True(t, ok)

	// A different size is a different file.
	require.NoError(t, os.WriteFile(path, []byte("retranscoded-longer"), 0o600))

	resized, ok := probeKeyFor(path)
	require.True(t, ok)
	assert.NotEqual(t, first, resized, "a size change must miss the cache")

	// So is a different modification time at the same size.
	stamp := time.Now().Add(2 * time.Hour)
	require.NoError(t, os.Chtimes(path, stamp, stamp))

	retimed, ok := probeKeyFor(path)
	require.True(t, ok)
	assert.NotEqual(t, resized, retimed, "an mtime change must miss the cache")

	// The same file is still the same key.
	again, ok := probeKeyFor(path)
	require.True(t, ok)
	assert.Equal(t, retimed, again)
}

// TestProbeKeySkipsMissingFiles keeps an unreadable path out of the cache, so it
// cannot collide with a replacement that appears at the same path later.
func TestProbeKeySkipsMissingFiles(t *testing.T) {
	t.Parallel()

	_, ok := probeKeyFor(filepath.Join(t.TempDir(), "absent.mkv"))
	assert.False(t, ok, "a file that cannot be stat'd must bypass the cache")
}

// TestProbeCacheEvictsAtCapacity proves the bound is enforced, by filling the
// cache and then overflowing it.
//
// Asserting only that the cache stayed within capacity would pass even if it
// never evicted anything, so this checks that the entry written past the limit
// is present and the entries from before it are gone.
func TestProbeCacheEvictsAtCapacity(t *testing.T) {
	t.Parallel()

	cache := probeCache{}
	identities := make([]probeIdentity, 0, probeCacheMaxEntries+1)

	dir := t.TempDir()

	for i := range probeCacheMaxEntries + 1 {
		path := filepath.Join(dir, "clip"+strconv.Itoa(i)+".mkv")
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

		identity, ok := probeKeyFor(path)
		require.True(t, ok)

		identities = append(identities, identity)

		cache.put(identity, MediaInfo{Duration: float64(i)})
	}

	overflowed := identities[len(identities)-1]
	earliest := identities[0]

	stored, ok := cache.get(overflowed)
	require.True(t, ok, "the entry written past the limit must be kept")
	assert.InDelta(t, float64(len(identities)-1), stored.Duration, 0.0005)

	_, ok = cache.get(earliest)
	assert.False(t, ok, "overflowing the cache must drop the earlier entries")

	cache.mu.Lock()

	size := len(cache.entries)
	cache.mu.Unlock()

	assert.LessOrEqual(t, size, probeCacheMaxEntries)
}

// TestProbeIdentityUnchangedDetectsReplacement covers the check Probe uses
// before caching, at the seam the race actually occurs at: an identity is taken,
// the file is replaced, and the second stat disagrees.
func TestProbeIdentityUnchangedDetectsReplacement(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(path, stamp, stamp))

	before, ok := probeKeyFor(path)
	require.True(t, ok)
	assert.True(t, probeIdentityUnchanged(path, before), "an untouched file is unchanged")

	// Replaced by a rename that preserves size and modification time, which is
	// the case the metadata key alone cannot see.
	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, stamp, stamp))
	require.NoError(t, os.Rename(replacement, path))

	after, ok := probeKeyFor(path)
	require.True(t, ok)
	assert.Equal(t, before.key, after.key, "the metadata key must match, or this proves nothing")
	assert.False(t, probeIdentityUnchanged(path, before), "a replaced file is not unchanged")
}

// TestProbeIdentityUnchangedDetectsRewrite covers an in-place rewrite, which
// keeps the inode and can preserve size while moving the modification time.
func TestProbeIdentityUnchangedDetectsRewrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	before, ok := probeKeyFor(path)
	require.True(t, ok)

	require.NoError(t, os.WriteFile(path, []byte("rewritten-and-longer"), 0o600))

	after, ok := probeKeyFor(path)
	require.True(t, ok)
	assert.NotEqual(t, before.key, after.key)
	assert.False(t, probeIdentityUnchanged(path, before), "a rewritten file is not unchanged")
}

// TestProbeIdentityUnchangedWhenFileVanished keeps a file removed mid-probe from
// being cached under an identity nothing resolves to any more.
func TestProbeIdentityUnchangedWhenFileVanished(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	before, ok := probeKeyFor(path)
	require.True(t, ok)

	require.NoError(t, os.Remove(path))

	assert.False(t, probeIdentityUnchanged(path, before))
}

// TestProbeIdentityUnchangedWithoutStat guards the nil case, so a caller that
// could not capture an identity never reports the file as unchanged.
func TestProbeIdentityUnchangedWithoutStat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	assert.False(t, probeIdentityUnchanged(path, probeIdentity{}))
	assert.False(t, probeIdentityUnchanged(filepath.Join(dir, "absent.mkv"), probeIdentity{}))
}

// TestProbeIdentityMissesReplacedFileWithEqualMetadata is the regression guard
// for file identity.
//
// Path, size, and modification time are all that a cache key carries, and a
// file retargeted in place, or replaced by a rename that preserves its
// metadata, keeps all three. Without the identity check the previous file's
// metadata would be served for the new content.
func TestProbeCacheMissesReplacedFileWithEqualMetadata(t *testing.T) {
	t.Parallel()

	cache := probeCache{}
	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")

	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(path, stamp, stamp))

	original, ok := probeKeyFor(path)
	require.True(t, ok)
	cache.put(original, MediaInfo{Width: 3840, VideoCodec: "hevc"})

	// Replace the file with different content of the same length, restoring the
	// modification time so the metadata key is identical.
	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, stamp, stamp))
	require.NoError(t, os.Rename(replacement, path))

	after, ok := probeKeyFor(path)
	require.True(t, ok)
	assert.Equal(
		t,
		original.key,
		after.key,
		"the metadata key must be unchanged, or this proves nothing",
	)

	cached, found := cache.get(after)
	assert.False(t, found, "a replaced file must miss the cache even when its metadata matches")
	assert.Equal(t, MediaInfo{}, cached)
}

// TestProbeCacheDoesNotShareAudioTracks proves callers cannot mutate each
// other's view through the cached slice.
func TestProbeCacheDoesNotShareAudioTracks(t *testing.T) {
	t.Parallel()

	cache := probeCache{}
	path := filepath.Join(t.TempDir(), "movie.mkv")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

	key, ok := probeKeyFor(path)
	require.True(t, ok)
	cache.put(key, MediaInfo{AudioTracks: []AudioTrack{{Index: 0, Codec: "aac"}}})

	first, ok := cache.get(key)
	require.True(t, ok)

	first.AudioTracks[0].Codec = "mutated"
	first.AudioTracks = append(first.AudioTracks, AudioTrack{Index: 9})

	second, ok := cache.get(key)
	require.True(t, ok)
	assert.Equal(
		t,
		"aac",
		second.AudioTracks[0].Codec,
		"a caller must not be able to rewrite the cached track",
	)
	assert.Len(t, second.AudioTracks, 1, "a caller must not be able to grow the cached slice")
}

func TestProbeCacheMissReturnsZero(t *testing.T) {
	t.Parallel()

	cache := probeCache{}

	info, ok := cache.get(probeIdentity{})

	assert.False(t, ok)
	assert.Equal(t, MediaInfo{}, info)
}
