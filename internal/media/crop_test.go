// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCropdetect(t *testing.T) {
	t.Parallel()

	const log = `[Parsed_cropdetect_0 @ 0x1] crop=1918:1078:2:0
[Parsed_cropdetect_0 @ 0x1] x1:0 x2:1919 y1:138 y2:941 w:1920 h:804 x:0 y:138 crop=1920:804:0:138
`

	crop, ok := ParseCropdetect(log)
	require.True(t, ok)
	assert.Equal(t, CropRect{Width: 1920, Height: 804, X: 0, Y: 138}, crop)
	assert.Equal(t, "crop=1920:804:0:138", crop.Filter())
}

func TestParseCropdetectEmpty(t *testing.T) {
	t.Parallel()

	crop, ok := ParseCropdetect("no crop here")
	assert.False(t, ok)
	assert.False(t, crop.Valid())
}

func TestCropRectTrims(t *testing.T) {
	t.Parallel()

	crop := CropRect{Width: 3840, Height: 1608, X: 0, Y: 276}
	assert.True(t, crop.Trims(3840, 2160))
	assert.False(t, CropRect{Width: 3840, Height: 2160, X: 0, Y: 0}.Trims(3840, 2160))
	assert.True(t, crop.Trims(0, 0))
}

func TestCropdetectArgsUsesEightBitLimit(t *testing.T) {
	t.Parallel()

	args := cropdetectArgs("ffmpeg", "/in.mkv", 10, 5)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "format=yuv420p")
	assert.Contains(t, joined, "limit=24/255")
}

func TestParseStreamSize(t *testing.T) {
	t.Parallel()

	const log = `Stream #0:0: Video: hevc (Main 10), yuv420p10le, 3840x2160 [SAR 1:1 DAR 16:9]`

	size := parseStreamSize(log)
	assert.Equal(t, 3840, size.width)
	assert.Equal(t, 2160, size.height)

	small := parseStreamSize(`Stream #0:0: Video: h264, yuv420p, 64x64`)
	assert.Equal(t, 64, small.width)
	assert.Equal(t, 64, small.height)
}

func TestCropdetectDuration(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, float64(cropdetectMaxSecs), cropdetectDuration(600), 0.001)
	assert.InDelta(t, 2.0, cropdetectDuration(2), 0.001)
	assert.InDelta(t, float64(cropdetectMaxSecs), cropdetectDuration(0), 0.001)
}
