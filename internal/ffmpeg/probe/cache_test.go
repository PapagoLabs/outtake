// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeProbeFixture(t *testing.T, contents string) (string, Key) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))

	identity, ok := KeyFor(path)
	require.True(t, ok)

	return path, identity
}

func writeProbeStub(t *testing.T, swap bool) string {
	t.Helper()

	const probeJSON = `{
  "format": {
    "duration": "12.5",
    "bit_rate": "8000",
    "format_name": "matroska"
  },
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "hevc",
      "width": 3840,
      "height": 2160,
      "color_transfer": "smpte2084"
    },
    {
      "index": 1,
      "codec_type": "audio",
      "codec_name": "eac3",
      "channels": 6,
      "tags": {
        "language": "eng",
        "title": "Surround"
      }
    }
  ]
}`

	if swap {
		return stubScript(t, probeSwapStubScript(probeJSON))
	}

	return stubScript(t, probeStubScript(probeJSON))
}

func TestProbeReturnsCachedResult(t *testing.T) {
	t.Parallel()

	path, identity := writeProbeFixture(t, "seeded")

	want := Info{
		Duration:      42 * time.Second,
		Width:         3840,
		Height:        2160,
		VideoCodec:    "hevc",
		ColorTransfer: "smpte2084",
		AudioTracks:   []Track{{Index: 0, Codec: "eac3"}},
	}
	probeResults.put(identity, want)

	got, err := Probe(t.Context(), "/nonexistent/ffprobe", path)
	require.NoError(t, err)
	assert.Equal(t, want.Duration, got.Duration)
	assert.Equal(t, want.Width, got.Width)
	assert.Equal(t, want.ColorTransfer, got.ColorTransfer)
	assert.Equal(t, want.AudioTracks, got.AudioTracks)
}

func TestProbeCachesStubbedResult(t *testing.T) {
	t.Parallel()

	path, identity := writeProbeFixture(t, "source")
	ffprobe := writeProbeStub(t, false)

	got, err := Probe(t.Context(), ffprobe, path)
	require.NoError(t, err)

	assert.Equal(t, 12500*time.Millisecond, got.Duration)
	assert.Equal(t, 3840, got.Width)
	assert.Equal(t, 2160, got.Height)
	assert.Equal(t, "hevc", got.VideoCodec)
	assert.Equal(t, "smpte2084", got.ColorTransfer)
	assert.Equal(t, "matroska", got.Format)
	require.Len(t, got.AudioTracks, 1)
	assert.Equal(t, "Surround", got.AudioTracks[0].Title)

	probeResults.mu.Lock()

	_, cached := probeResults.entries[identity.key]
	probeResults.mu.Unlock()

	assert.True(t, cached, "an untouched file must still be cached after the identity re-check")

	second, err := Probe(t.Context(), ffprobe, path)
	require.NoError(t, err)
	assert.Equal(t, got, second)
}

func TestProbeSkipsCachingWhenFileChangesDuringProbe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(target, []byte("original-content"), 0o600))

	before, ok := KeyFor(target)
	require.True(t, ok)

	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, before.key.mtime, before.key.mtime))

	_, err := Probe(t.Context(), writeProbeStub(t, true), target)
	require.NoError(t, err, "the stub answers regardless of what the file contains")

	after, ok := KeyFor(target)
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

	probeResults.mu.Lock()

	_, cached := probeResults.entries[before.key]
	probeResults.mu.Unlock()

	assert.False(
		t,
		cached,
		"a file that changed mid-probe must not be cached under the identity taken before it",
	)
}

func TestProbeKeyTracksFileIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))

	first, ok := KeyFor(path)
	require.True(t, ok)

	require.NoError(t, os.WriteFile(path, []byte("retranscoded-longer"), 0o600))

	resized, ok := KeyFor(path)
	require.True(t, ok)
	assert.NotEqual(t, first, resized, "a size change must miss the cache")

	stamp := time.Now().Add(2 * time.Hour)
	require.NoError(t, os.Chtimes(path, stamp, stamp))

	retimed, ok := KeyFor(path)
	require.True(t, ok)
	assert.NotEqual(t, resized, retimed, "an mtime change must miss the cache")

	again, ok := KeyFor(path)
	require.True(t, ok)
	assert.Equal(t, retimed, again)
}

func TestProbeKeySkipsMissingFiles(t *testing.T) {
	t.Parallel()

	_, ok := KeyFor(filepath.Join(t.TempDir(), "absent.mkv"))
	assert.False(t, ok, "a file that cannot be stat'd must bypass the cache")
}

func TestProbeCacheEvictsAtCapacity(t *testing.T) {
	t.Parallel()

	entries := probeCache{}
	identities := make([]Key, 0, cacheMaxEntries+1)

	dir := t.TempDir()

	for i := range cacheMaxEntries + 1 {
		path := filepath.Join(dir, "clip"+strconv.Itoa(i)+".mkv")
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

		identity, ok := KeyFor(path)
		require.True(t, ok)

		identities = append(identities, identity)

		entries.put(identity, Info{Duration: time.Duration(i) * time.Second})
	}

	overflowed := identities[len(identities)-1]
	earliest := identities[0]

	stored, ok := entries.get(overflowed)
	require.True(t, ok, "the entry written past the limit must be kept")
	assert.Equal(t, time.Duration(len(identities)-1)*time.Second, stored.Duration)

	_, ok = entries.get(earliest)
	assert.False(t, ok, "overflowing the cache must drop the earlier entries")

	entries.mu.Lock()

	size := len(entries.entries)
	entries.mu.Unlock()

	assert.LessOrEqual(t, size, cacheMaxEntries)
}

func TestProbeIdentityUnchangedDetectsReplacement(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(path, stamp, stamp))

	before, ok := KeyFor(path)
	require.True(t, ok)
	assert.True(t, identityUnchanged(path, before), "an untouched file is unchanged")

	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, stamp, stamp))
	require.NoError(t, os.Rename(replacement, path))

	after, ok := KeyFor(path)
	require.True(t, ok)
	assert.Equal(t, before.key, after.key, "the metadata key must match, or this proves nothing")
	assert.False(t, identityUnchanged(path, before), "a replaced file is not unchanged")
}

func TestProbeIdentityUnchangedDetectsRewrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	before, ok := KeyFor(path)
	require.True(t, ok)

	require.NoError(t, os.WriteFile(path, []byte("rewritten-and-longer"), 0o600))

	after, ok := KeyFor(path)
	require.True(t, ok)
	assert.NotEqual(t, before.key, after.key)
	assert.False(t, identityUnchanged(path, before), "a rewritten file is not unchanged")
}

func TestProbeIdentityUnchangedWhenFileVanished(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	before, ok := KeyFor(path)
	require.True(t, ok)

	require.NoError(t, os.Remove(path))

	assert.False(t, identityUnchanged(path, before))
}

func TestProbeIdentityUnchangedWithoutStat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	assert.False(t, identityUnchanged(path, Key{}))
	assert.False(t, identityUnchanged(filepath.Join(dir, "absent.mkv"), Key{}))
}

func TestProbeCacheMissesReplacedFileWithEqualMetadata(t *testing.T) {
	t.Parallel()

	entries := probeCache{}
	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")

	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(path, stamp, stamp))

	original, ok := KeyFor(path)
	require.True(t, ok)
	entries.put(original, Info{Width: 3840, VideoCodec: "hevc"})

	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, stamp, stamp))
	require.NoError(t, os.Rename(replacement, path))

	after, ok := KeyFor(path)
	require.True(t, ok)
	assert.Equal(
		t,
		original.key,
		after.key,
		"the metadata key must be unchanged, or this proves nothing",
	)

	cached, found := entries.get(after)
	assert.False(t, found, "a replaced file must miss the cache even when its metadata matches")
	assert.Equal(t, Info{}, cached)
}

func TestProbeCacheDoesNotShareAudioTracks(t *testing.T) {
	t.Parallel()

	entries := probeCache{}
	path := filepath.Join(t.TempDir(), "movie.mkv")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

	key, ok := KeyFor(path)
	require.True(t, ok)
	entries.put(key, Info{AudioTracks: []Track{{Index: 0, Codec: "aac"}}})

	first, ok := entries.get(key)
	require.True(t, ok)

	first.AudioTracks[0].Codec = "mutated"
	first.AudioTracks = append(first.AudioTracks, Track{Index: 9})

	second, ok := entries.get(key)
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

	entries := probeCache{}

	info, ok := entries.get(Key{})

	assert.False(t, ok)
	assert.Equal(t, Info{}, info)
}
