// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package crop

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

	parsed, ok := ParseCropdetect(log)
	require.True(t, ok)
	assert.Equal(t, CropRect{Width: 1920, Height: 804, X: 0, Y: 138}, parsed)
	assert.Equal(t, "crop=1920:804:0:138", parsed.Filter())
}

func TestParseCropdetectEmpty(t *testing.T) {
	t.Parallel()

	parsed, ok := ParseCropdetect("no crop here")
	assert.False(t, ok)
	assert.False(t, parsed.Valid())
}

func TestCropRectTrims(t *testing.T) {
	t.Parallel()

	parsed := CropRect{Width: 3840, Height: 1608, X: 0, Y: 276}
	assert.True(t, parsed.Trims(3840, 2160))
	assert.False(t, CropRect{Width: 3840, Height: 2160, X: 0, Y: 0}.Trims(3840, 2160))
	assert.True(t, parsed.Trims(0, 0))
}

func TestCropdetectArgsUsesEightBitLimit(t *testing.T) {
	t.Parallel()

	args := DetectArgs("ffmpeg", "/in.mkv", 10, 5)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "format=yuv420p")
	assert.Contains(t, joined, "limit=24/255")
}

func TestParseStreamSize(t *testing.T) {
	t.Parallel()

	const log = `Stream #0:0: Video: hevc (Main 10), yuv420p10le, 3840x2160 [SAR 1:1 DAR 16:9]`

	size := ParseStreamSize(log)
	assert.Equal(t, 3840, size.Width)
	assert.Equal(t, 2160, size.Height)

	small := ParseStreamSize(`Stream #0:0: Video: h264, yuv420p, 64x64`)
	assert.Equal(t, 64, small.Width)
	assert.Equal(t, 64, small.Height)
}

func TestCropdetectDuration(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, float64(cropdetectMaxSecs), cropdetectDuration(600), 0.001)
	assert.InDelta(t, 2.0, cropdetectDuration(2), 0.001)
	assert.InDelta(t, float64(cropdetectMaxSecs), cropdetectDuration(0), 0.001)
}
