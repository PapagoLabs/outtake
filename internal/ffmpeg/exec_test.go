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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
)

type encodeFixture struct {
	input  string
	output string
}

var encodeStubPath = writeEncodeStubs()

func argvStubScript() string {
	return "#!/bin/sh\n" +
		"for arg in \"$@\"; do last=$arg; done\n" +
		"if [ \"$last\" = \"-\" ]; then exit 0; fi\n" +
		"for arg in \"$@\"; do printf '%s\\n' \"$arg\" >> \"${last}.argv\"; done\n"
}

func writeEncodeStubs() string {
	dir := stubDir()
	if dir == "" {
		return ""
	}

	const sdrProbe = `{
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
}`

	for _, script := range []string{argvStubScript(), probeStubScript(sdrProbe)} {
		writeStubDirect(script)
	}

	return stubPath(dir, argvStubScript())
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

	_, statErr := os.Stat("/bin/sh")
	if statErr != nil {
		t.Skip("a POSIX shell is required for the ffmpeg stub")
	}

	if encodeStubPath == "" {
		t.Skip("unable to create a directory for the ffmpeg stub")
	}

	return NewExecFFmpeg(encodeStubPath, stubScript(t, probeStubScript(probeJSON)))
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

	data, err := os.ReadFile(output + ".argv")
	require.NoError(t, err, "the stub records the argv it was invoked with")

	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// writePassStub builds an ffmpeg stub whose pass writes payload to stderr.
func writePassStub(t *testing.T, stderr string) string {
	t.Helper()

	return stubScript(t, plainStubScript(stderr))
}

// writeSwappingPassStub builds an ffmpeg stub that swaps the source file while
// the pass runs, so a cache keyed on the pre-pass identity must be rejected.
func writeSwappingPassStub(t *testing.T, stderr string) string {
	t.Helper()

	return stubScript(t, swapStubScript(stderr))
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

func TestExecFFmpeg_UsesDefaultTimeout(t *testing.T) {
	t.Parallel()

	execFFmpeg := NewExecFFmpeg("sleep", "sleep")

	assert.Equal(t, DefaultFFmpegTimeout(), execFFmpeg.timeout)
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
