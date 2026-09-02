// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"os/exec"
	"strconv"
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
	assert.Contains(t, args, "0:a:1")
	assert.Contains(t, args, "-pix_fmt")
	assert.Contains(t, args, pixelFormatYUV420P)
	assert.Contains(t, args, "-vf")
	assert.Contains(t, args, scaleFilter(OutputWidth2160p, scaleFlagsLanczos))
	assert.NotContains(t, args, "128k")
	assert.NotContains(t, args, "crop=")
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

func TestPreviewEncodeArgs(t *testing.T) {
	t.Parallel()

	args := previewEncodeArgs("ffmpeg", "/in.mkv", "/out.mp4", 10, 5, 1, CropRect{})

	assert.Contains(t, args, "-pix_fmt")
	assert.Contains(t, args, pixelFormatYUV420P)
	assert.Contains(t, args, "-preset")
	assert.Contains(t, args, previewPreset)
	assert.Contains(t, args, "-crf")
	assert.Contains(t, args, strconv.Itoa(previewCRF))
	assert.Contains(t, args, strconv.Itoa(previewAudioKbps)+"k")
	assert.Contains(t, args, scaleFilter(previewMaxWidth, scaleFlagsFast))
	assert.NotContains(t, args, "320k")
}

func TestPreviewDuration(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 10.0, previewDuration(10), 0.001)
	assert.InDelta(t, float64(previewMaxSecs), previewDuration(600), 0.001)
	assert.InDelta(t, 0.0, previewDuration(0), 0.001)
}

func TestExecFFmpeg_ExtractScreenshot_Args(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("echo", "echo")
	ctx := t.Context()

	err := ff.ExtractScreenshot(ctx, "/tmp/input.mp4", "/tmp/screenshot.jpg", 120.0)
	require.NoError(t, err)
}
