// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
)

func TestProbeMissingFile(t *testing.T) {
	t.Parallel()

	_, err := Probe(t.Context(), "ffprobe", "/nonexistent/file.mp4")
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
	assert.Equal(t, 120500*time.Millisecond, info.Duration)
	assert.Equal(t, int64(5000000), info.BitRate)
	assert.Equal(t, "h264", info.VideoCodec)
	assert.Equal(t, "aac", info.AudioCodec)
	require.Len(t, info.AudioTracks, 1)
	assert.Equal(t, "aac", info.AudioTracks[0].Codec)
	assert.Equal(t, 1920, info.Width)
	assert.Equal(t, 1080, info.Height)
	assert.Empty(t, info.ColorTransfer)
}

func TestParseProbeOutput_HDRTransfer(t *testing.T) {
	t.Parallel()

	data := []byte(`{
		"format": {"duration": "10.0", "bit_rate": "1000", "format_name": "matroska"},
		"streams": [
			{"codec_type": "video", "codec_name": "hevc", "width": 3840, "height": 2160,
				"color_transfer": "smpte2084"}
		]
	}`)

	info, err := parseProbeOutput(data)
	require.NoError(t, err)
	assert.Equal(t, "smpte2084", info.ColorTransfer)
	assert.True(t, clip.IsHDRTransfer(info.ColorTransfer))
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

func TestParseProbeOutput_DurationIsExact(t *testing.T) {
	t.Parallel()

	tests := map[string]time.Duration{
		"":            0,
		"0.000000":    0,
		"12.500000":   12500 * time.Millisecond,
		"120.500000":  120500 * time.Millisecond,
		"3600.123456": 3600123456 * time.Microsecond,
		"N/A":         0,
	}

	for payload, want := range tests {
		data := []byte(`{"format": {"duration": "` + payload + `"}, "streams": []}`)

		info, err := parseProbeOutput(data)
		require.NoError(t, err)
		assert.Equal(t, want, info.Duration, "duration %q", payload)
	}
}

// TestParseProbeOutputReadsTheDolbyVisionRecord covers the Dolby Vision
// configuration record ffprobe lists in a video stream's side data, and which
// profiles have no base layer an export can show correctly.
func TestParseProbeOutputReadsTheDolbyVisionRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sideData  string
		want      DolbyVision
		reshaping bool
	}{
		{
			name:      "no record",
			sideData:  `[]`,
			want:      DolbyVision{Present: false, Profile: 0, BaseLayerCompatibility: 0},
			reshaping: false,
		},
		{
			name: "profile 5",
			sideData: `[{"side_data_type":"DOVI configuration record","dv_profile":5,` +
				`"dv_bl_signal_compatibility_id":0}]`,
			want:      DolbyVision{Present: true, Profile: 5, BaseLayerCompatibility: 0},
			reshaping: true,
		},
		{
			name: "profile 8.1 with an HDR10 base layer",
			sideData: `[{"side_data_type":"DOVI configuration record","dv_profile":8,` +
				`"dv_bl_signal_compatibility_id":1}]`,
			want:      DolbyVision{Present: true, Profile: 8, BaseLayerCompatibility: 1},
			reshaping: false,
		},
		{
			name: "profile 7 from a UHD Blu-ray",
			sideData: `[{"side_data_type":"DOVI configuration record","dv_profile":7,` +
				`"dv_bl_signal_compatibility_id":6}]`,
			want:      DolbyVision{Present: true, Profile: 7, BaseLayerCompatibility: 6},
			reshaping: false,
		},
		{
			name: "AV1 profile 10 with no compatible base layer",
			sideData: `[{"side_data_type":"DOVI configuration record","dv_profile":10,` +
				`"dv_bl_signal_compatibility_id":0}]`,
			want:      DolbyVision{Present: true, Profile: 10, BaseLayerCompatibility: 0},
			reshaping: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			data := []byte(`{"format":{"duration":"10.0"},"streams":[{"codec_type":"video",` +
				`"codec_name":"hevc","color_transfer":"smpte2084","side_data_list":` +
				test.sideData + `}]}`)

			info, err := parseProbeOutput(data)
			require.NoError(t, err)

			assert.Equal(t, test.want, info.DolbyVision)
			assert.Equal(t, test.reshaping, info.NeedsDolbyVisionReshaping())
		})
	}
}

// TestParseProbeOutputReadsTheLightLevels covers HDR10 light levels as
// ffprobe prints them for a container that carries them, and the peak they
// give: MaxCLL first, the mastering display's peak when MaxCLL is unknown.
func TestParseProbeOutputReadsTheLightLevels(t *testing.T) {
	t.Parallel()

	data := []byte(`{"format":{"duration":"3.0"},"streams":[{"codec_type":"video",` +
		`"codec_name":"hevc","color_transfer":"smpte2084","side_data_list":[` +
		`{"side_data_type":"Content light level metadata","max_content":1000,"max_average":400},` +
		`{"side_data_type":"Mastering display metadata","red_x":"17/25","min_luminance":"1/10000",` +
		`"max_luminance":"1000/1"}]}]}`)

	info, err := parseProbeOutput(data)
	require.NoError(t, err)

	assert.InDelta(t, 1000.0, info.MaxCLLNits, 0.0001)
	assert.InDelta(t, 1000.0, info.MasteringMaxNits, 0.0001)

	peak, ok := info.PeakNits()
	require.True(t, ok)
	assert.InDelta(t, 1000.0, peak, 0.0001)

	_, ok = Info{}.PeakNits()
	assert.False(t, ok, "a stream without HDR10 metadata has no known peak")

	peak, ok = Info{MasteringMaxNits: 4000}.PeakNits()
	require.True(t, ok)
	assert.InDelta(t, 4000.0, peak, 0.0001, "the mastering peak stands in for an unknown MaxCLL")
}

// TestParseRationalReadsFfprobeValues covers ffprobe's numerator/denominator
// values and the malformed ones it never trusts.
func TestParseRationalReadsFfprobeValues(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 1000.0, parseRational("10000000/10000"), 0.0001)
	assert.InDelta(t, 1000.0, parseRational("1000"), 0.0001)
	assert.Zero(t, parseRational(""))
	assert.Zero(t, parseRational("1000/0"))
	assert.Zero(t, parseRational("x/1"))
}
