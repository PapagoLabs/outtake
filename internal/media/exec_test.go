// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecFFmpeg_Run_CommandNotFound(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("/nonexistent/ffmpeg", "/nonexistent/ffprobe")
	err := ff.run(t.Context(), 0, "/nonexistent/ffmpeg", "-version")
	assert.Error(t, err)
}

func TestExecFFmpeg_Run_Timeout(t *testing.T) {
	t.Parallel()

	_, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}

	ff := NewExecFFmpeg("sleep", "sleep")
	ff.SetTimeout(100 * time.Millisecond)

	err = ff.run(t.Context(), 0, "sleep", "10")
	assert.Error(t, err)
}

func TestExecFFmpeg_ExtractClip_Args(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("echo", "echo")
	ctx := t.Context()

	err := ff.ExtractClip(
		ctx,
		"/tmp/input.mp4",
		"/tmp/output.mp4",
		10.5,
		30.0,
		QualityPresets[ClipQualityHigh],
		1,
		CropRect{},
	)
	require.NoError(t, err)
}

func TestClipEncodeArgs(t *testing.T) {
	t.Parallel()

	args := clipEncodeArgs(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		QualityPresets[ClipQualityHigh],
		1,
		CropRect{},
	)

	assert.Contains(t, args, "-ac")
	assert.Contains(t, args, "2")
	assert.Contains(t, args, "-b:a")
	assert.Contains(t, args, "320k")
	assert.Contains(t, args, "0:a:1?")
	assert.Contains(t, args, "-pix_fmt")
	assert.Contains(t, args, pixelFormatYUV420P)
	assert.Contains(t, args, "-vf")
	assert.Contains(t, args, scaleFilter(OutputWidth2160p, scaleFlagsLanczos))
	assert.NotContains(t, args, "128k")
	assert.NotContains(t, args, "crop=")
	assert.NotContains(t, args, "libplacebo")
	assert.NotContains(t, args, "zscale=tin=smpte2084")
	assert.NotContains(t, args, "write_colr")
}

func TestClipEncodeArgsCropsBlackBars(t *testing.T) {
	t.Parallel()

	crop := CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	args := clipEncodeArgs(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		QualityPresets[ClipQualityHigh],
		1,
		crop,
	)

	assert.Contains(t, args, crop.Filter()+","+scaleFilter(OutputWidth2160p, scaleFlagsLanczos))
}

func TestClipEncodeArgsWebSafeColor(t *testing.T) {
	t.Parallel()

	preset := QualityPresets[ClipQualityHigh]

	preset.WebSafeColor = true

	args := clipEncodeArgs(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		preset,
		1,
		CropRect{},
	)

	joined := strings.Join(args, " ")
	wantFilter := webSafeToneMapFilter(transferPQAlias, defaultWebSafePeak) +
		"," + scaleFilter(OutputWidth2160p, scaleFlagsLanczos)
	assert.Contains(t, args, wantFilter)
	assert.Contains(t, joined, "zscale=tin=smpte2084")
	assert.Contains(t, joined, "tonemap=tonemap=hable")
	assert.Contains(t, args, "-color_primaries")
	assert.Contains(t, args, "bt709")
	assert.Contains(t, args, "-color_trc")
	assert.Contains(t, args, "iec61966-2-1")
	assert.Contains(t, args, webSafeMovFlags)
	assert.NotContains(t, joined, "libplacebo")
	assert.NotContains(t, args, "-init_hw_device")
}

func TestPreviewEncodeArgs(t *testing.T) {
	t.Parallel()

	args := previewEncodeArgs(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		1,
		CropRect{},
		QualityPreset{},
	)

	assert.Contains(t, args, "-pix_fmt")
	assert.Contains(t, args, pixelFormatYUV420P)
	assert.Contains(t, args, "-preset")
	assert.Contains(t, args, previewPreset)
	assert.Contains(t, args, "-crf")
	assert.Contains(t, args, strconv.Itoa(previewCRF))
	assert.Contains(t, args, strconv.Itoa(previewAudioKbps)+"k")
	assert.Contains(t, args, scaleFilter(previewMaxWidth, scaleFlagsFast))
	assert.NotContains(t, args, "320k")
	assert.NotContains(t, strings.Join(args, " "), "tonemap=tonemap=hable")
}

func TestPreviewEncodeArgsWebSafeColor(t *testing.T) {
	t.Parallel()

	args := previewEncodeArgs(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		1,
		CropRect{},
		QualityPreset{WebSafeColor: true},
	)

	joined := strings.Join(args, " ")
	wantFilter := webSafeToneMapFilter(transferPQAlias, defaultWebSafePeak) +
		"," + scaleFilter(previewMaxWidth, scaleFlagsFast)
	assert.Contains(t, args, wantFilter)
	assert.Contains(t, joined, "tonemap=tonemap=hable")
	assert.Contains(t, args, "-color_primaries")
	assert.Contains(t, args, "bt709")
	assert.Contains(t, args, "-color_trc")
	assert.Contains(t, args, "iec61966-2-1")
	assert.Contains(t, args, webSafeMovFlags)
	assert.Contains(t, joined, scaleFilter(previewMaxWidth, scaleFlagsFast))
	assert.NotContains(t, joined, "libplacebo")
}

func TestPreviewDuration(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 10.0, previewDuration(10), 0.001)
	assert.InDelta(t, float64(previewMaxSecs), previewDuration(600), 0.001)
	assert.InDelta(t, 0.0, previewDuration(0), 0.001)
}

func TestScaleFilterForcesEvenWidth(t *testing.T) {
	t.Parallel()

	assert.Contains(t, scaleFilter(1920, scaleFlagsLanczos), "trunc(min(1920,iw)/2)*2")
	assert.Contains(t, scaleFilter(1920, scaleFlagsLanczos), "h=-2")
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
	ff := NewExecFFmpeg(ffmpegPath, "ffprobe")
	ff.SetTimeout(30 * time.Second)

	err = ff.ExtractGIF(t.Context(), src, out, 0, 0.5, 160, 10, CropRect{})
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Greater(t, len(data), 6)
	assert.Equal(t, "GIF", string(data[:3]))
}

func TestExecFFmpeg_ExtractScreenshot_Args(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("echo", "echo")
	ctx := t.Context()

	err := ff.ExtractScreenshot(ctx, "/tmp/input.mp4", "/tmp/screenshot.jpg", 120.0, CropRect{})
	require.NoError(t, err)
}

func TestGIFPaletteFilterCropsBlackBars(t *testing.T) {
	t.Parallel()

	crop := CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	want := crop.Filter() +
		",fps=10,scale=480:-2:flags=lanczos,format=yuv420p,palettegen=stats_mode=diff"

	assert.Equal(t, want, gifPaletteFilter(480, 10, crop))
}

func TestGIFEncodeFilterCropsBlackBars(t *testing.T) {
	t.Parallel()

	crop := CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	want := "[0:v]" + crop.Filter() +
		",fps=10,scale=480:-2:flags=lanczos,format=yuv420p[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5"

	assert.Equal(t, want, gifEncodeFilter(480, 10, crop))
}

func TestGIFPaletteFilterOmitsCropWhenEmpty(t *testing.T) {
	t.Parallel()

	got := gifPaletteFilter(480, 10, CropRect{})
	want := "fps=10,scale=480:-2:flags=lanczos,format=yuv420p,palettegen=stats_mode=diff"

	assert.Equal(t, want, got)
	assert.NotContains(t, got, "crop=")
}

func TestGIFPaletteArgsLimitsInput(t *testing.T) {
	t.Parallel()

	args := gifPaletteArgs("ffmpeg", "/in.mkv", "/p.png", 10, 5, "vf")
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

	args := gifEncodeArgs("ffmpeg", "/in.mkv", "/p.png", "/out.gif", 10, 5, "fc")
	firstInput := slices.Index(args, inputFlag)
	dur := slices.Index(args, durationFlag)

	assert.Greater(t, firstInput, dur)
	assert.Equal(t, "/in.mkv", args[firstInput+1])
	assert.Equal(t, "/p.png", args[firstInput+3])
	assert.Contains(t, args, anFlag)
	assert.NotContains(t, args[firstInput:], durationFlag)
}

func TestScreenshotEncodeArgsCropsBlackBars(t *testing.T) {
	t.Parallel()

	crop := CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	args := screenshotEncodeArgs("ffmpeg", "/in.mkv", "/out.jpg", 12, crop)

	assert.Contains(t, args, "-vf")
	assert.Contains(t, args, crop.Filter())
}

func TestScreenshotEncodeArgsOmitsCropWhenEmpty(t *testing.T) {
	t.Parallel()

	args := screenshotEncodeArgs("ffmpeg", "/in.mkv", "/out.jpg", 12, CropRect{})

	assert.NotContains(t, args, "-vf")
	assert.NotContains(t, args, "crop=")
}
