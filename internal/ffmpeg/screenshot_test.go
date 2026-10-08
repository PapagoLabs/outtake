// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
)

func TestExecFFmpeg_ExtractScreenshot_MissingInput(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("ffmpeg", "ffprobe")
	err := ff.ExtractScreenshot(
		t.Context(),
		"/nonexistent/input.mp4",
		"/tmp/screenshot.jpg",
		100,
		crop.CropRect{},
	)
	assert.Error(t, err)
}

func TestExecFFmpeg_ExtractScreenshot_Args(t *testing.T) {
	t.Parallel()

	fixture := newEncodeFixture(t, "screenshot.jpg")
	ff := NewExecFFmpeg(ffmpegtest.Install(t, ffmpegtest.Stub{Output: "frame"}), "unused")

	err := ff.ExtractScreenshot(
		t.Context(),
		fixture.input,
		fixture.output,
		2*time.Minute,
		crop.CropRect{},
	)
	require.NoError(t, err)

	published, err := os.ReadFile(fixture.output)
	require.NoError(t, err, "the staged still is moved into place")
	assert.Equal(t, "frame", string(published))
}

func TestExtractScreenshotResolvesRelativePaths(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	dir := t.TempDir()
	relDir, err := filepath.Rel(cwd, dir)
	require.NoError(t, err)
	require.False(t, filepath.IsAbs(relDir), "the input under test has to be relative")

	input := filepath.Join(relDir, "in.mkv")
	output := filepath.Join(relDir, "out.jpg")

	require.NoError(t, os.WriteFile(input, []byte("source"), 0o600))

	logPath := filepath.Join(dir, "argv")
	runner := NewExecFFmpeg(
		ffmpegtest.Install(t, ffmpegtest.Stub{ArgvFile: logPath, Output: "frame"}),
		"unused",
	)

	require.NoError(t, runner.ExtractScreenshot(
		t.Context(),
		input,
		output,
		time.Second,
		crop.CropRect{},
	))

	recorded, err := os.ReadFile(logPath)
	require.NoError(t, err)

	args := strings.Split(strings.TrimSuffix(string(recorded), "\n"), "\n")
	assert.Contains(t, args, filepath.Join(dir, "in.mkv"))

	staged := args[len(args)-1]
	assert.Equal(t, dir, filepath.Dir(staged), "the still is staged beside its absolute output")
	assert.FileExists(t, filepath.Join(dir, "out.jpg"))
}

func TestScreenshotEncodeArgsCropsBlackBars(t *testing.T) {
	t.Parallel()

	rect := crop.CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	args := screenshotEncodeArgs("ffmpeg", "/in.mkv", "/out.jpg", 12*time.Second, rect, "")

	assert.Contains(t, args, "-vf")
	assert.Contains(t, args, rect.Filter())
}

func TestScreenshotEncodeArgsOmitsCropWhenEmpty(t *testing.T) {
	t.Parallel()

	args := screenshotEncodeArgs(
		"ffmpeg",
		"/in.mkv",
		"/out.jpg",
		12*time.Second,
		crop.CropRect{},
		"",
	)

	assert.NotContains(t, args, "-vf")
	assert.NotContains(t, args, "crop=")
}

// TestScreenshotEncodeArgsWriteOneImage covers the image muxer flag: a still
// is one file, so ffmpeg is told not to expect a numbered sequence.
func TestScreenshotEncodeArgsWriteOneImage(t *testing.T) {
	t.Parallel()

	args := screenshotEncodeArgs("ffmpeg", "/in.mkv", "/out.jpg", time.Second, crop.CropRect{}, "")

	assert.Equal(t, "/out.jpg", args[len(args)-1])
	assert.Contains(t, strings.Join(args, " "), updateFlag+" 1")
}
