// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
)

func TestExecFFmpeg_ExtractGIF_MissingInput(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("ffmpeg", "ffprobe")
	err := ff.ExtractGIF(
		t.Context(),
		"/nonexistent/input.mp4",
		"/tmp/output.gif",
		0,
		5,
		480,
		10,
		crop.CropRect{},
		clip.QualityPreset{},
	)
	assert.Error(t, err)
}

func TestExtractGIFWritesFile(t *testing.T) {
	t.Parallel()

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	makeCmd := exec.CommandContext(
		t.Context(),
		ffmpegPath,
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=size=320x240:rate=24:duration=1",
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		src,
	)
	require.NoError(t, makeCmd.Run())

	out := filepath.Join(dir, "out.gif")

	err = NewExecFFmpeg(ffmpegPath, "ffprobe").
		ExtractGIF(
			t.Context(),
			src, out, 0, 500*time.Millisecond, 160, 10, crop.CropRect{}, clip.QualityPreset{},
		)
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Greater(t, len(data), 6)
	assert.Equal(t, "GIF", string(data[:3]))
}

func TestGIFPaletteFilterCropsBlackBars(t *testing.T) {
	t.Parallel()

	rect := crop.CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	want := rect.Filter() +
		",fps=10,scale='trunc(480/2)*2':-2:flags=lanczos,format=yuv420p,palettegen=stats_mode=diff"

	assert.Equal(t, want, gifPaletteFilter(gifFrames{width: 480, fps: 10, rect: rect}))
}

func TestGIFEncodeFilterCropsBlackBars(t *testing.T) {
	t.Parallel()

	rect := crop.CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	want := "[0:v]" + rect.Filter() +
		",fps=10,scale='trunc(480/2)*2':-2:flags=lanczos,format=yuv420p[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5"

	assert.Equal(t, want, gifEncodeFilter(gifFrames{width: 480, fps: 10, rect: rect}))
}

func TestGIFPaletteFilterOmitsCropWhenEmpty(t *testing.T) {
	t.Parallel()

	got := gifPaletteFilter(gifFrames{width: 480, fps: 10, rect: crop.CropRect{}})
	want := "fps=10,scale='trunc(480/2)*2':-2:flags=lanczos,format=yuv420p,palettegen=stats_mode=diff"

	assert.Equal(t, want, got)
	assert.NotContains(t, got, "crop=")
}

func TestGIFPaletteArgsLimitsInput(t *testing.T) {
	t.Parallel()

	args := gifPaletteArgs("ffmpeg", "/in.mkv", "/p.png", 10*time.Second, 5*time.Second, "vf")
	ss := slices.Index(args, ssFlag)
	dur := slices.Index(args, durationFlag)
	in := slices.Index(args, inputFlag)

	assert.Greater(t, dur, ss)
	assert.Greater(t, in, dur)
	assert.Contains(t, args, anFlag)
	assert.Contains(t, args, updateFlag)
	assert.Contains(t, args, framesFlag)
}

func TestGIFEncodeArgsLimitsInput(t *testing.T) {
	t.Parallel()

	args := gifEncodeArgs(
		"ffmpeg", "/in.mkv", "/p.png", "/out.gif",
		10*time.Second, 5*time.Second, "fc",
	)
	firstInput := slices.Index(args, inputFlag)
	dur := slices.Index(args, durationFlag)

	assert.Greater(t, firstInput, dur)
	assert.Equal(t, "/in.mkv", args[firstInput+1])
	assert.Equal(t, "/p.png", args[firstInput+3])
	assert.Contains(t, args, anFlag)
	assert.NotContains(t, args[firstInput:], durationFlag)
}
