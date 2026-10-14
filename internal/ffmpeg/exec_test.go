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

	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
)

type encodeFixture struct {
	input  string
	output string
}

// runSeparator ends each run in a recorded argv log.
const runSeparator = "--- run ---"

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

// TestExecFFmpeg_Run_CommandNotFound covers a binary that is not where the
// configuration says.
//
// The run reports that FFmpeg never started.
func TestExecFFmpeg_Run_CommandNotFound(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("/nonexistent/ffmpeg", "/nonexistent/ffprobe")
	err := ff.run(t.Context(), 0, "/nonexistent/ffmpeg", "-version")
	require.ErrorIs(t, err, ErrNotStarted)
}

// TestExecFFmpeg_Run_ReportsItsOwnDeadline covers a run stopped by its
// deadline.
//
// The error names the limit and the setting that raises it.
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
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotStarted, "a stopped run is not a binary that cannot start")
}

// TestThreadedBoundsEveryPool covers the thread bound added to an argv.
//
// The filter graphs are bounded first, each input's decoder before it, the
// encoders after the last input, and libx265's pool in its parameters. No
// bound, or an argv without an input, leaves the argv as it is.
func TestThreadedBoundsEveryPool(t *testing.T) {
	t.Parallel()

	gif := []string{"ffmpeg", "-y", "-i", "in.mkv", "-i", "palette.png", "-an", "out.gif"}
	hevc := []string{
		"ffmpeg",
		"-i",
		"in.mkv",
		"-c:v",
		"libx265",
		"-x265-params",
		"hdr10-opt=1",
		"out.mp4",
	}

	for name, test := range map[string]struct {
		args    []string
		want    []string
		threads int
	}{
		"no bound": {args: gif, threads: 0, want: gif},
		"no input": {args: []string{"ffmpeg", "-version"}, threads: 4, want: []string{"ffmpeg", "-version"}},
		"two inputs": {args: gif, threads: 4, want: []string{
			"ffmpeg", "-filter_threads", "4", "-filter_complex_threads", "4",
			"-y", "-threads", "4", "-i", "in.mkv", "-threads", "4", "-i", "palette.png",
			"-threads", "4", "-an", "out.gif",
		}},
		"x265": {args: hevc, threads: 2, want: []string{
			"ffmpeg", "-filter_threads", "2", "-filter_complex_threads", "2",
			"-threads", "2", "-i", "in.mkv", "-threads", "2",
			"-c:v", "libx265", "-x265-params", "hdr10-opt=1:pools=2", "out.mp4",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, threaded(test.args, test.threads))
		})
	}
}

// TestBoundRunsCarryTheThreadCount covers a runner given a thread bound.
//
// Every run a clip, a preview, a GIF, a screenshot, and crop detection start
// carries the bound for its filters, each decoder, and the encoders, and the
// HEVC encode bounds libx265's pool as well.
func TestBoundRunsCarryTheThreadCount(t *testing.T) {
	t.Parallel()

	const threads = "3"

	argvLog := filepath.Join(t.TempDir(), "argv.log")

	runner := hdrEncodeExec(t)

	runner.ffmpegPath = ffmpegtest.Install(t, ffmpegtest.Stub{
		ArgvFile:      argvLog,
		ArgvSeparator: runSeparator,
		Output:        "rendered",
	})
	runner = runner.WithThreads(3)

	keepHDR := highPreset

	keepHDR.PreserveHDR = true

	fixture := newEncodeFixture(t, "clip.mp4")
	start, length := 10*time.Second, 5*time.Second

	require.NoError(t, runner.ExtractClip(
		t.Context(), fixture.input, fixture.output, start, length, keepHDR, 1, crop.CropRect{},
	))
	require.NoError(t, runner.ExtractPreview(
		t.Context(), fixture.input, filepath.Join(filepath.Dir(fixture.output), "preview.mp4"),
		start, length, 1, crop.CropRect{}, highPreset,
	))
	require.NoError(t, runner.ExtractGIF(
		t.Context(), fixture.input, filepath.Join(filepath.Dir(fixture.output), "clip.gif"),
		start, length, 480, 10, crop.CropRect{},
	))
	require.NoError(t, runner.ExtractScreenshot(
		t.Context(), fixture.input, filepath.Join(filepath.Dir(fixture.output), "still.jpg"),
		start, crop.CropRect{},
	))

	_, _ = runner.DetectCrop(t.Context(), fixture.input, start, length)

	runs := recordedRuns(t, argvLog)
	require.GreaterOrEqual(t, len(runs), 6, "every path ran ffmpeg")

	pooled := 0

	for _, run := range runs {
		assert.Equal(
			t,
			[]string{"-filter_threads", threads, "-filter_complex_threads", threads},
			run[:4],
		)

		last := -1

		for index, arg := range run {
			if arg == inputFlag {
				assert.Equal(
					t,
					[]string{"-threads", threads},
					run[index-2:index],
					"the decoder of %s",
					run[index+1],
				)

				last = index
			}

			if arg == flagX265Params {
				assert.True(t, strings.HasSuffix(run[index+1], ":pools="+threads))

				pooled++
			}
		}

		require.NotEqual(t, -1, last)
		assert.Equal(t, []string{"-threads", threads}, run[last+2:last+4], "the encoders")
	}

	assert.Equal(t, 1, pooled, "the HEVC clip bounds libx265's pool")
}

// recordedRuns splits a recorded argv log into its runs, each without the
// binary the fake records no line for.
//
// Parameters:
//   - t: The test that reads the log.
//   - path: The log the fake wrote.
//
// Returns:
//   - runs: Each run's argv, in the order they ran.
func recordedRuns(t *testing.T, path string) [][]string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var runs [][]string

	for chunk := range strings.SplitSeq(string(data), runSeparator+"\n") {
		if chunk = strings.TrimSuffix(chunk, "\n"); chunk != "" {
			runs = append(runs, strings.Split(chunk, "\n"))
		}
	}

	return runs
}
