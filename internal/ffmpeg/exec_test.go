// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
)

type encodeFixture struct {
	input  string
	output string
}

func newEncodeFixture(t *testing.T, name string) encodeFixture {
	t.Helper()

	dir := t.TempDir()
	input := filepath.Join(dir, "source.mkv")

	require.NoError(t, os.WriteFile(input, []byte("source"), 0o600))

	return encodeFixture{input: input, output: filepath.Join(dir, name)}
}

func newEncodeExec(t *testing.T, probeJSON string) *ExecFFmpeg {
	t.Helper()

	return NewExecFFmpeg(
		ffmpegtest.Install(t, ffmpegtest.Stub{ArgvBesideOutput: true, Output: "rendered"}),
		probeStub(t, probeJSON),
	)
}

func sdrEncodeExec(t *testing.T) *ExecFFmpeg {
	t.Helper()

	return newEncodeExec(t, `{
  "format": {
    "duration": "12.5",
    "bit_rate": "8000",
    "format_name": "matroska"
  },
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "h264",
      "width": 1920,
      "height": 1080,
      "color_transfer": "bt709"
    },
    {
      "index": 1,
      "codec_type": "audio",
      "codec_name": "aac",
      "channels": 2
    }
  ]
}`)
}

func hdrEncodeExec(t *testing.T) *ExecFFmpeg {
	t.Helper()

	return newEncodeExec(t, `{
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
}`)
}

func readRecordedArgv(t *testing.T, output string) []string {
	t.Helper()

	// The render writes to a staging file beside output, and the stub records
	// its argv beside that file.
	recorded, err := filepath.Glob(filepath.Join(filepath.Dir(output), stagingPrefix+"*.argv"))
	require.NoError(t, err)
	require.Len(t, recorded, 1, "the stub records the argv it was invoked with")

	data, err := os.ReadFile(recorded[0])
	require.NoError(t, err)

	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// newPassSource writes a source file and returns its path and cache identity.
func newPassSource(t *testing.T) (string, probe.Key) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	identity, ok := probe.KeyFor(path)
	require.True(t, ok)

	return path, identity
}

func TestExecFFmpeg_Run_CommandNotFound(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("/nonexistent/ffmpeg", "/nonexistent/ffprobe")
	err := ff.run(t.Context(), 0, "/nonexistent/ffmpeg", "-version")
	assert.Error(t, err)
}

// TestExecFFmpeg_Run_ReportsItsOwnDeadline covers a run stopped by its
// deadline: the error names the limit and the setting that raises it.
func TestExecFFmpeg_Run_ReportsItsOwnDeadline(t *testing.T) {
	t.Parallel()

	_, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}

	err = NewExecFFmpeg("sleep", "sleep").WithTimeout(50*time.Millisecond).
		run(t.Context(), 0, "sleep", "10")

	require.ErrorIs(t, err, ErrTimeout)
	assert.Contains(t, err.Error(), "ffmpeg-timeout-sec")
	assert.Contains(t, err.Error(), "50ms")
}

func TestExecFFmpeg_Run_StopsWithContext(t *testing.T) {
	t.Parallel()

	_, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = NewExecFFmpeg("sleep", "sleep").run(ctx, 0, "sleep", "10")
	assert.Error(t, err)
}
