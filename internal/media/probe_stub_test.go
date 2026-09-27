// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProbeJSON is a minimal ffprobe payload, in the shape parseProbeOutput
// expects. Probe only needs the binary to print this and exit zero, so the test
// does not need a probe installed.
const stubProbeJSON = `{
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

// writeProbeStub returns a stand-in for ffprobe that answers with a fixed
// payload, so Probe can be exercised without a real ffprobe.
//
// When swap is set the stub first moves a sibling named swapReplacementName onto
// the file under probe. A rename gives the target a new identity while carrying
// the replacement's modification time and size across, so the metadata key is
// unchanged and the file identity is the only thing that can tell the two apart.
// Copying instead would rewrite the contents in place and leave the identity
// alone, which is a different scenario and one the metadata key already catches.
//
// Parameters:
//   - t: Test context.
//   - swap: Whether the stub replaces the file under probe before answering.
//
// Returns:
//   - path: Path of the stub to use as the ffprobe binary.
func writeProbeStub(t *testing.T, swap bool) string {
	t.Helper()

	if swap {
		return stubScript(t, probeSwapStubScript(stubProbeJSON))
	}

	return stubScript(t, probeStubScript(stubProbeJSON))
}

// TestProbeCachesStubbedResult covers the whole Probe path, cache write
// included, without needing a probe installed.
//
// It also confirms the re-stat before caching does not quietly disable the cache
// for an ordinary, untouched file.
func TestProbeCachesStubbedResult(t *testing.T) {
	t.Parallel()

	path, identity := writeProbeFixture(t, "source")
	execFFmpeg := NewExecFFmpeg("unused", writeProbeStub(t, false))

	got, err := execFFmpeg.Probe(t.Context(), path)
	require.NoError(t, err)

	assert.InDelta(t, 12.5, got.Duration, 0.0005)
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

	// A second probe is answered from the cache, so the parsed result matches.
	second, err := execFFmpeg.Probe(t.Context(), path)
	require.NoError(t, err)
	assert.Equal(t, got, second)
}

// TestProbeSkipsCachingWhenFileChangesDuringProbe covers the re-stat before
// caching.
//
// The stub replaces the file between the identity being taken and the result
// being stored, which is the window the guard closes. The replacement is the
// same length and carries the same modification time, so the metadata key is
// unchanged and the file identity is the only thing that can tell the two
// apart. Nothing may be cached, because the identity recorded before the probe
// no longer describes the file at that path.
func TestProbeSkipsCachingWhenFileChangesDuringProbe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(target, []byte("original-content"), 0o600))

	before, ok := probeKeyFor(target)
	require.True(t, ok)

	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, before.key.mtime, before.key.mtime))

	execFFmpeg := NewExecFFmpeg("unused", writeProbeStub(t, true))

	_, err := execFFmpeg.Probe(t.Context(), target)
	require.NoError(t, err, "the stub answers regardless of what the file contains")

	// The replacement must have landed without disturbing the metadata key, or
	// the assertion below would pass for the wrong reason.
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

	probeResults.mu.Lock()

	_, cached := probeResults.entries[before.key]
	probeResults.mu.Unlock()

	assert.False(
		t,
		cached,
		"a file that changed mid-probe must not be cached under the identity taken before it",
	)
}
