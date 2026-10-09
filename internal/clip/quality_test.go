// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDefaultPresetIsThe1080pProfile covers the settings a render falls back
// on: the 1080p built-in's, which migration 010 stores, converting to SDR.
func TestDefaultPresetIsThe1080pProfile(t *testing.T) {
	t.Parallel()

	assert.Equal(t, QualityPreset{
		CRF:         20,
		Preset:      "medium",
		AudioKbps:   192,
		MaxWidth:    OutputWidth1080p,
		PreserveHDR: false,
	}, DefaultPreset)
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
				Preset:    "slow",
				AudioKbps: 320,
				MaxWidth:  OutputWidth2160p,
			}, true
		}

		return QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}, false
	}

	assert.Equal(t, 16, ResolvePreset("archive", lookup).CRF)
	assert.Equal(t, DefaultPreset, ResolvePreset("high", lookup),
		"an old built-in id is not known without a stored profile")
	assert.Equal(t, DefaultPreset, ResolvePreset("missing", lookup))
	assert.Equal(t, DefaultPreset, ResolvePreset("archive", nil))
}

func TestNormalizePreset(t *testing.T) {
	t.Parallel()

	high := QualityPreset{
		CRF:         18,
		Preset:      "slow",
		AudioKbps:   256,
		MaxWidth:    OutputWidth2160p,
		PreserveHDR: false,
	}

	assert.Equal(
		t,
		DefaultPreset,
		NormalizePreset(QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}),
	)
	assert.Equal(t, high, NormalizePreset(high))
}

func TestIsHDRTransfer(t *testing.T) {
	t.Parallel()

	assert.True(t, IsPQTransfer("smpte2084"))
	assert.True(t, IsHLGTransfer("arib-std-b67"))
	assert.True(t, IsHDRTransfer("smpte2084"))
	assert.False(t, IsHDRTransfer("bt709"))
	assert.False(t, IsHDRTransfer(""))
}
