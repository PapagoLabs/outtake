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

	pq := ToneMapFilter(clip.TransferPQAlias, 3.8471)
	assert.Contains(t, pq, "zscale=tin=smpte2084")
	assert.Contains(t, pq, "tonemap=tonemap=hable:desat=0:peak=3.8471")
	assert.Contains(t, pq, "iec61966-2-1")
	assert.NotContains(t, pq, "libplacebo")

	hlg := ToneMapFilter(clip.TransferHLGAlias, 0)
	assert.Contains(t, hlg, "zscale=tin=arib-std-b67")
	assert.Contains(t, hlg, "peak=4.0000")
}
