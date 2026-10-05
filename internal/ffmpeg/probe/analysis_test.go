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

	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
)

func TestWindowKeyRoundsLikeFFmpeg(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		windowKey(10*time.Second),
		windowKey(10*time.Second+100*time.Microsecond),
		"a sub-millisecond difference must not create a second entry",
	)
	assert.NotEqual(
		t,
		windowKey(10*time.Second),
		windowKey(10*time.Second+2*time.Millisecond),
	)
	assert.Equal(t, int64(12_345), windowKey(12345*time.Millisecond))
	assert.Equal(t, windowKey(0), windowKey(100*time.Microsecond))
}

func TestAnalysisCacheEvictsAtCapacity(t *testing.T) {
	t.Parallel()

	entries := &analysisCache{}
	dir := t.TempDir()
	keys := make([]analysisKey, 0, analysisCacheMaxEntries+1)

	for i := range analysisCacheMaxEntries + 1 {
		path := filepath.Join(dir, "clip"+strconv.Itoa(i)+".mkv")
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

		identity, ok := KeyFor(path)
		require.True(t, ok)

		key := analysisKeyFor(identity, kindPeak, 0, 0)

		keys = append(keys, key)
		entries.put(key, analysisValue{peak: float64(i)}, identity.file)
	}

	overflowed := keys[len(keys)-1]
	earliest := keys[0]

	got, ok := entries.get(overflowed, nil)
	require.True(t, ok, "the entry written past the limit must be kept")
	assert.InDelta(t, float64(len(keys)-1), got.peak, 0.001)

	_, ok = entries.get(earliest, nil)
	assert.False(t, ok, "overflowing the cache must drop the earlier entries")

	entries.mu.Lock()

	size := len(entries.entries)
	entries.mu.Unlock()

	assert.LessOrEqual(t, size, analysisCacheMaxEntries)
}

func TestKeyWithoutFileNeverConsultsTheCache(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	identity, ok := KeyFor(path)
	require.True(t, ok)

	StoreCrop(path, identity, 0, 0, crop.CropRect{Width: 1920, Height: 804})
	StorePeak(path, identity, 0, 0, 158)

	rect, cached := identity.CachedCrop(0, 0)
	assert.True(t, cached)
	assert.Equal(t, crop.CropRect{Width: 1920, Height: 804}, rect)

	peak, cached := identity.CachedPeak(0, 0)
	assert.True(t, cached)
	assert.InDelta(t, 158.0, peak, 0.001)

	unstatable, ok := KeyFor(filepath.Join(t.TempDir(), "absent.mkv"))
	assert.False(t, ok)

	_, cached = unstatable.CachedCrop(0, 0)
	assert.False(t, cached, "a file that cannot be stat'd must bypass the cache")

	_, cached = unstatable.CachedPeak(0, 0)
	assert.False(t, cached, "a file that cannot be stat'd must bypass the cache")
}
