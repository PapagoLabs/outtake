// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecFFmpeg_Probe_MissingFile(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("ffprobe", "ffmpeg")
	_, err := ff.Probe(t.Context(), "/nonexistent/file.mp4")
	assert.Error(t, err)
}

func TestExecFFmpeg_ExtractClip_MissingInput(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("ffmpeg", "ffprobe")
	err := ff.ExtractClip(
		t.Context(),
		"/nonexistent/input.mp4",
		"/tmp/output.mp4",
		0,
		10,
		QualityPresets[ClipQualityMedium],
		0,
		CropRect{},
	)
	assert.Error(t, err)
}

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
		CropRect{},
	)
	assert.Error(t, err)
}

func TestExecFFmpeg_ExtractScreenshot_MissingInput(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("ffmpeg", "ffprobe")
	err := ff.ExtractScreenshot(
		t.Context(),
		"/nonexistent/input.mp4",
		"/tmp/screenshot.jpg",
		100,
		CropRect{},
	)
	assert.Error(t, err)
}





func TestFormatDuration(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "10.500", formatDuration(10.5))
	assert.Equal(t, "0.000", formatDuration(0))
	assert.Equal(t, "3600.000", formatDuration(3600))
}

func TestDefaultFFmpegTimeout(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 30*60*1000000000, int(DefaultFFmpegTimeout()))
}
