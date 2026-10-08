// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package tonemap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
)

func TestNitsFromLimitedY(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 0.0, NitsFromLimitedY(16), 0.01)
	assert.InDelta(t, 201.0, NitsFromLimitedY(143), 15)
	assert.InDelta(t, 385.0, NitsFromLimitedY(158), 25)
}

func TestPeakFromNits(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 1.0, PeakFromNits(50), 0.001)
	assert.InDelta(t, 3.847, PeakFromNits(384.7), 0.02)
}

func TestParseSignalstatsYMax(t *testing.T) {
	t.Parallel()

	log := "lavfi.signalstats.YMAX=143.0\nlavfi.signalstats.YMAX=158.0\n"
	ymax, ok := ParseSignalstatsYMax(log)
	require.True(t, ok)
	assert.InDelta(t, 158.0, ymax, 0.001)

	_, ok = ParseSignalstatsYMax("no stats")
	assert.False(t, ok)
}

func TestToneMapFilter(t *testing.T) {
	t.Parallel()

	pq := ToneMapFilter(clip.TransferPQAlias, 3.8471, TransferBT709)
	assert.Contains(t, pq, "zscale=tin=smpte2084")
	assert.Contains(t, pq, "tonemap=tonemap=mobius:desat=0:peak=3.8471")
	assert.Contains(t, pq, "zscale=t=bt709:m=bt709:p=bt709:r=tv", "video is encoded BT.709")
	assert.NotContains(t, pq, "hable")
	assert.NotContains(t, pq, "libplacebo")

	still := ToneMapFilter(clip.TransferPQAlias, 3.8471, TransferSRGB)
	assert.Contains(t, still, "zscale=t=iec61966-2-1:m=bt709", "stills and GIFs are encoded sRGB")

	hlg := ToneMapFilter(clip.TransferHLGAlias, 0, TransferBT709)
	assert.Contains(t, hlg, "zscale=tin=arib-std-b67")
	assert.Contains(t, hlg, "peak=4.0000")
}
