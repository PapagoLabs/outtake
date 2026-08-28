// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		ClipQualityMedium,
	)
	assert.Error(t, err)
}

func TestExecFFmpeg_ExtractGIF_MissingInput(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("ffmpeg", "ffprobe")
	err := ff.ExtractGIF(t.Context(), "/nonexistent/input.mp4", "/tmp/output.gif", 0, 5, 480, 10)
	assert.Error(t, err)
}

func TestExecFFmpeg_ExtractScreenshot_MissingInput(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("ffmpeg", "ffprobe")
	err := ff.ExtractScreenshot(t.Context(), "/nonexistent/input.mp4", "/tmp/screenshot.jpg", 100)
	assert.Error(t, err)
}

func TestParseProbeOutput(t *testing.T) {
	t.Parallel()

	data := []byte(`{
		"format": {
			"duration": "120.500000",
			"bit_rate": "5000000",
			"format_name": "mov,mp4,m4a,3gp,3g2,mj2"
		},
		"streams": [
			{"codec_type": "video", "codec_name": "h264", "width": 1920, "height": 1080},
			{"codec_type": "audio", "codec_name": "aac"}
		]
	}`)

	info, err := parseProbeOutput(data)
	require.NoError(t, err)
	assert.InEpsilon(t, 120.5, info.Duration, 0.01)
	assert.Equal(t, int64(5000000), info.BitRate)
	assert.Equal(t, "h264", info.VideoCodec)
	assert.Equal(t, "aac", info.AudioCodec)
	assert.Equal(t, 1920, info.Width)
	assert.Equal(t, 1080, info.Height)
}

func TestParseProbeOutput_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := parseProbeOutput([]byte("not json"))
	assert.Error(t, err)
}

func TestQualityPresets(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 28, QualityPresets[ClipQualityLow].CRF)
	assert.Equal(t, 23, QualityPresets[ClipQualityMedium].CRF)
	assert.Equal(t, 18, QualityPresets[ClipQualityHigh].CRF)
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
