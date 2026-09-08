// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.True(t, isHDRTransfer(info.ColorTransfer))
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
