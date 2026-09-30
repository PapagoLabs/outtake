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

// TestSourceQuality pins the source label shown beside a media title.
//
// It is derived from the probed height and transfer, because a container tag can
// claim a transfer the pixel data does not carry.
func TestSourceQuality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		info    MediaInfo
		want    string
		wantHDR bool
	}{
		{
			name:    "a 4k pq source reads as 4k hdr10",
			info:    MediaInfo{Height: 2160, ColorTransfer: transferPQ},
			want:    "4K HDR10",
			wantHDR: true,
		},
		{
			name:    "a 1080p hlg source reads as 1080p hlg",
			info:    MediaInfo{Height: 1080, ColorTransfer: transferHLG},
			want:    "1080p HLG",
			wantHDR: true,
		},
		{
			name:    "an sdr source carries no transfer name",
			info:    MediaInfo{Height: 1080, ColorTransfer: nameBT709},
			want:    "1080p",
			wantHDR: false,
		},
		{
			name:    "a short pq alias is still hdr",
			info:    MediaInfo{Height: 720, ColorTransfer: transferPQAlias},
			want:    "720p HDR10",
			wantHDR: true,
		},
		{
			name:    "an unknown height drops the resolution class",
			info:    MediaInfo{ColorTransfer: transferPQ},
			want:    "HDR10",
			wantHDR: true,
		},
		{
			name:    "an unprobed source has no label",
			info:    MediaInfo{},
			want:    "",
			wantHDR: false,
		},
		{
			name:    "an odd height keeps its own number",
			info:    MediaInfo{Height: 576},
			want:    "576p",
			wantHDR: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, SourceQuality(test.info))
			assert.Equal(t, test.wantHDR, IsHDRSource(test.info.ColorTransfer))
		})
	}
}
