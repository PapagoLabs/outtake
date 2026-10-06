// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
)

// cropdetectLog is the stderr a stubbed cropdetect pass writes.
const cropdetectLog = "[Parsed_cropdetect_0 @ 0x1] crop=3840:1608:0:216\n" +
	"Stream #0:0: Video: hevc, yuv420p, 3840x2160 [SAR 1:1 DAR 16:9]\n"

// cropdetectedRect is the rectangle cropdetectLog parses into.
var cropdetectedRect = crop.CropRect{Width: 3840, Height: 1608, X: 0, Y: 216}

// cropExec builds an executor whose cropdetect pass is stubbed.
func cropExec(t *testing.T) *ExecFFmpeg {
	t.Helper()

	return NewExecFFmpeg(passStub(t, cropdetectLog), "unused")
}

// blindExec is an executor that cannot run anything, so a cache miss surfaces
// as an error instead of a second identical answer.
func blindExec() *ExecFFmpeg {
	return NewExecFFmpeg("/nonexistent/ffmpeg", "unused")
}

func cropCached(identity probe.Key, start, window time.Duration) bool {
	_, found := identity.CachedCrop(start, window)

	return found
}

func TestDetectCropCachesByFileAndWindow(t *testing.T) {
	t.Parallel()

	path, identity := newPassSource(t)
	execFFmpeg := cropExec(t)

	first, err := execFFmpeg.DetectCrop(t.Context(), path, 10*time.Second, 30*time.Second)
	require.NoError(t, err)
	require.Equal(t, cropdetectedRect, first,
		"the stubbed cropdetect log must be parsed into a trimming rectangle")

	assert.True(
		t,
		cropCached(identity, 10*time.Second, crop.SampleDuration(30*time.Second)),
		"a successful pass must be cached",
	)

	second, err := blindExec().DetectCrop(t.Context(), path, 10*time.Second, 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestDetectCropKeyTracksSeekAndWindow(t *testing.T) {
	t.Parallel()

	path, _ := newPassSource(t)
	execFFmpeg := cropExec(t)

	_, err := execFFmpeg.DetectCrop(t.Context(), path, 10*time.Second, 30*time.Second)
	require.NoError(t, err)

	blind := blindExec()

	_, err = blind.DetectCrop(t.Context(), path, 12*time.Second, 30*time.Second)
	require.Error(t, err, "a different seek must not be served from the cache")

	_, err = blind.DetectCrop(t.Context(), path, 10, 1500*time.Millisecond)
	require.Error(t, err, "a shorter sampling window must not be served from the cache")
}

func TestDetectCropSharesEntryAcrossClampedWindows(t *testing.T) {
	t.Parallel()

	path, _ := newPassSource(t)
	execFFmpeg := cropExec(t)

	first, err := execFFmpeg.DetectCrop(t.Context(), path, 10*time.Second, 3*time.Second)
	require.NoError(t, err)

	second, err := blindExec().
		DetectCrop(t.Context(), path, 10*time.Second, 600*time.Second)
	require.NoError(t, err, "a longer clip must reuse the clamped entry")
	assert.Equal(t, first, second)
}

func TestDetectCropMissesReplacedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0o600))

	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(path, stamp, stamp))

	_, err := cropExec(t).DetectCrop(t.Context(), path, 10*time.Second, 30*time.Second)
	require.NoError(t, err)

	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, stamp, stamp))
	require.NoError(t, os.Rename(replacement, path))

	_, err = blindExec().DetectCrop(t.Context(), path, 10*time.Second, 30*time.Second)
	require.Error(t, err, "a replaced file must miss the cache")
}

func TestDetectCropSkipsCachingWhenFileChangesDuringProbe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(target, []byte("original-content"), 0o600))

	before, err := os.Stat(target)
	require.NoError(t, err)

	identity, ok := probe.KeyFor(target)
	require.True(t, ok)

	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, before.ModTime(), before.ModTime()))

	execFFmpeg := NewExecFFmpeg(swappingPassStub(t, cropdetectLog), "unused")

	rect, err := execFFmpeg.DetectCrop(t.Context(), target, 10*time.Second, 30*time.Second)
	require.NoError(t, err, "the stub answers regardless of what the file contains")
	require.NotZero(t, rect.Height, "the crop log must still be parsed")

	after, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(
		t,
		before.ModTime(),
		after.ModTime(),
		"the metadata key must be unchanged, or this proves nothing",
	)
	assert.Equal(
		t,
		before.Size(),
		after.Size(),
		"the metadata key must be unchanged, or this proves nothing",
	)
	assert.False(t, os.SameFile(before, after), "the file at target must now be a different file")

	assert.False(
		t,
		cropCached(identity, 10*time.Second, crop.SampleDuration(30*time.Second)),
		"a file that changed mid-pass must not be cached under the identity taken before it",
	)
}
