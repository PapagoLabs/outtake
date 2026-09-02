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
		QualityPresets[ClipQualityMedium],
		0,
		CropRect{},
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
	require.Len(t, info.AudioTracks, 1)
	assert.Equal(t, "aac", info.AudioTracks[0].Codec)
	assert.Equal(t, 1920, info.Width)
	assert.Equal(t, 1080, info.Height)
}

func TestParseProbeOutput_AudioTracks(t *testing.T) {
	t.Parallel()

	data := []byte(`{
		"format": {"duration": "10.0", "bit_rate": "1000", "format_name": "matroska"},
		"streams": [
			{"index": 0, "codec_type": "video", "codec_name": "hevc", "width": 3840, "height": 2160},
			{"index": 1, "codec_type": "audio", "codec_name": "dts", "channels": 8,
				"tags": {"language": "eng", "name": "DTS:X 7.1"}},
			{"index": 2, "codec_type": "audio", "codec_name": "aac", "channels": 2,
				"tags": {"language": "eng", "title": "Commentary"}}
		]
	}`)

	info, err := parseProbeOutput(data)
	require.NoError(t, err)
	require.Len(t, info.AudioTracks, 2)
	assert.Equal(t, 0, info.AudioTracks[0].Index)
	assert.Equal(t, "dts", info.AudioTracks[0].Codec)
	assert.Equal(t, "DTS:X 7.1", info.AudioTracks[0].Title)
	assert.Equal(t, 8, info.AudioTracks[0].Channels)
	assert.Equal(t, 1, info.AudioTracks[1].Index)
	assert.Equal(t, "Commentary", info.AudioTracks[1].Title)
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
	assert.Equal(t, 128, QualityPresets[ClipQualityLow].AudioKbps)
	assert.Equal(t, 192, QualityPresets[ClipQualityMedium].AudioKbps)
	assert.Equal(t, 320, QualityPresets[ClipQualityHigh].AudioKbps)
	assert.Equal(t, OutputWidth720p, QualityPresets[ClipQualityLow].MaxWidth)
	assert.Equal(t, OutputWidth1080p, QualityPresets[ClipQualityMedium].MaxWidth)
	assert.Equal(t, OutputWidth2160p, QualityPresets[ClipQualityHigh].MaxWidth)
}

func TestNormalizeOutputWidth(t *testing.T) {
	t.Parallel()

	assert.True(t, ValidOutputWidth(OutputWidth2160p))
	assert.False(t, ValidOutputWidth(1000))
	assert.Equal(t, OutputWidth1080p, NormalizeOutputWidth(0))
	assert.Equal(t, OutputWidth2160p, NormalizeOutputWidth(OutputWidth2160p))
	assert.Equal(t, "4K", OutputWidthLabel(OutputWidth2160p))
	assert.Equal(t, "1080p", OutputWidthLabel(OutputWidth1080p))
}

func TestResolvePreset(t *testing.T) {
	t.Parallel()

	lookup := func(id string) (QualityPreset, bool) {
		if id == "archive" {
			return QualityPreset{
				CRF:       16,
				Preset:    QualityPresets[ClipQualityHigh].Preset,
				AudioKbps: 320,
				MaxWidth:  OutputWidth2160p,
			}, true
		}

		return QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}, false
	}

	assert.Equal(t, 16, ResolvePreset("archive", lookup).CRF)
	assert.Equal(t, QualityPresets[ClipQualityHigh], ResolvePreset("high", lookup))
	assert.Equal(t, QualityPresets[ClipQualityMedium], ResolvePreset("missing", lookup))
	assert.Equal(t, QualityPresets[ClipQualityMedium], ResolvePreset("archive", nil))
}

func TestNormalizePreset(t *testing.T) {
	t.Parallel()

	high := QualityPresets[ClipQualityHigh]

	assert.Equal(
		t,
		QualityPresets[ClipQualityMedium],
		NormalizePreset(QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}),
	)
	assert.Equal(t, high, NormalizePreset(high))
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
