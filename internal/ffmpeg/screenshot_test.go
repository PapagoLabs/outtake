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

	ff := NewExecFFmpeg("echo", "echo")
	ctx := t.Context()

	err := ff.ExtractScreenshot(
		ctx, "/tmp/input.mp4", "/tmp/screenshot.jpg", 2*time.Minute, crop.CropRect{},
	)
	require.NoError(t, err)
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
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" +
		strings.ReplaceAll(logPath, "'", `'\''`) + "'\n"
	runner := NewExecFFmpeg(stubScript(t, script), "unused")

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
	assert.Contains(t, args, filepath.Join(dir, "out.jpg"))
}

func TestScreenshotEncodeArgsCropsBlackBars(t *testing.T) {
	t.Parallel()

	rect := crop.CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	args := screenshotEncodeArgs("ffmpeg", "/in.mkv", "/out.jpg", 12*time.Second, rect)

	assert.Contains(t, args, "-vf")
	assert.Contains(t, args, rect.Filter())
}

func TestScreenshotEncodeArgsOmitsCropWhenEmpty(t *testing.T) {
	t.Parallel()

	args := screenshotEncodeArgs("ffmpeg", "/in.mkv", "/out.jpg", 12*time.Second, crop.CropRect{})

	assert.NotContains(t, args, "-vf")
	assert.NotContains(t, args, "crop=")
}
