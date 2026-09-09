// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package quality

import (
	"slices"
	"strconv"
)

// ClipQuality represents a built-in clip quality identifier.
type ClipQuality string

// QualityPreset represents ffmpeg clip encode settings.
type QualityPreset struct {
	CRF       int
	Preset    string
	AudioKbps int
	MaxWidth  int
	// WebSafeColor tone-maps HDR to Rec.709 when true.
	WebSafeColor bool
}

const (
	// ClipQualityLow is the low quality preset.
	ClipQualityLow ClipQuality = "low"
	// ClipQualityMedium is the medium quality preset.
	ClipQualityMedium ClipQuality = "medium"
	// ClipQualityHigh is the high quality preset.
	ClipQualityHigh  ClipQuality = "high"
	crfLowQuality                = 28
	crfMediumQuality             = 23
	crfHighQuality               = 18
	// MinCRF is the lowest allowed libx264 CRF.
	MinCRF = 0
	// MaxCRF is the highest allowed libx264 CRF.
	MaxCRF = 51
	// MinAudioKbps is the lowest allowed AAC bitrate.
	MinAudioKbps = 64
	// MaxAudioKbps is the highest allowed AAC bitrate.
	MaxAudioKbps       = 640
	audioKbpsLow       = 128
	audioKbpsMedium    = 192
	audioKbpsHigh      = 320
	channelsMono       = 1
	channelsStereo     = 2
	channelsSurround51 = 6
	channelsSurround71 = 8
	// OutputWidth720p is 1280px wide.
	OutputWidth720p = 1280
	// OutputWidth1080p is 1920px wide.
	OutputWidth1080p = 1920
	// OutputWidth1440p is 2560px wide.
	OutputWidth1440p = 2560
	// OutputWidth2160p is 3840px wide (4K).
	OutputWidth2160p = 3840
)

// EncoderPresets lists valid libx264 -preset values from fastest to slowest.
var EncoderPresets = []string{
	"ultrafast",
	"superfast",
	"veryfast",
	"faster",
	"fast",
	"medium",
	"slow",
	"slower",
	"veryslow",
}

// QualityPresets maps built-in quality identifiers to presets.
var QualityPresets = map[ClipQuality]QualityPreset{
	ClipQualityLow: {
		CRF:       crfLowQuality,
		Preset:    "veryfast",
		AudioKbps: audioKbpsLow,
		MaxWidth:  OutputWidth720p,
	},
	ClipQualityMedium: {
		CRF:       crfMediumQuality,
		Preset:    "medium",
		AudioKbps: audioKbpsMedium,
		MaxWidth:  OutputWidth1080p,
	},
	ClipQualityHigh: {
		CRF:       crfHighQuality,
		Preset:    "slow",
		AudioKbps: audioKbpsHigh,
		MaxWidth:  OutputWidth2160p,
	},
}

// OutputWidths lists selectable clip export widths.
var OutputWidths = []int{
	OutputWidth720p,
	OutputWidth1080p,
	OutputWidth1440p,
	OutputWidth2160p,
}

// ValidEncoderPreset reports whether name is a supported libx264 preset.
//
// Parameters:
//   - name: Human-readable quality preset name.
//
// Returns:
//   - ok: True when name is a supported libx264 preset.
func ValidEncoderPreset(name string) bool {
	return slices.Contains(EncoderPresets, name)
}

// ValidCRF reports whether crf is in the libx264 range.
//
// Parameters:
//   - crf: Constant rate factor for x264/x265 (lower is higher quality).
//
// Returns:
//   - ok: True when crf is in the libx264 range.
func ValidCRF(crf int) bool {
	return crf >= MinCRF && crf <= MaxCRF
}

// ValidAudioKbps reports whether kbps is in the allowed AAC range.
//
// Parameters:
//   - kbps: Target video bitrate in kilobits per second.
//
// Returns:
//   - ok: True when kbps is in the allowed AAC range.
func ValidAudioKbps(kbps int) bool {
	return kbps >= MinAudioKbps && kbps <= MaxAudioKbps
}

// NormalizePreset fills missing or invalid encode settings with Medium.
//
// Parameters:
//   - preset: Encode quality profile (CRF, bitrate, scale).
//
// Returns:
//   - qualityPreset: Result of NormalizePreset.
func NormalizePreset(preset QualityPreset) QualityPreset {
	if !ValidCRF(preset.CRF) || !ValidEncoderPreset(preset.Preset) {
		return QualityPresets[ClipQualityMedium]
	}

	if !ValidAudioKbps(preset.AudioKbps) {
		preset.AudioKbps = QualityPresets[ClipQualityMedium].AudioKbps
	}

	preset.MaxWidth = NormalizeOutputWidth(preset.MaxWidth)

	return preset
}

// ValidOutputWidth reports whether width is a supported export width.
//
// Parameters:
//   - width: Target max width in pixels (even dimensions).
//
// Returns:
//   - ok: True when width is a supported export width.
func ValidOutputWidth(width int) bool {
	return slices.Contains(OutputWidths, width)
}

// NormalizeOutputWidth returns width if supported, otherwise 1080p.
//
// Parameters:
//   - width: Target max width in pixels (even dimensions).
//
// Returns:
//   - n: The width if supported, otherwise 1080p.
func NormalizeOutputWidth(width int) int {
	if ValidOutputWidth(width) {
		return width
	}

	return OutputWidth1080p
}

// OutputWidthLabel is the UI label for an export width.
//
// Parameters:
//   - width: Target max width in pixels (even dimensions).
//
// Returns:
//   - value: Result value. Zero or empty when unavailable.
func OutputWidthLabel(width int) string {
	switch width {
	case OutputWidth720p:
		return "720p"
	case OutputWidth1080p:
		return "1080p"
	case OutputWidth1440p:
		return "1440p"
	case OutputWidth2160p:
		return "4K"
	default:
		return strconv.Itoa(width)
	}
}

// ResolvePreset maps a stored quality id onto ffmpeg settings.
//
// Parameters:
//   - qualityID: Quality id.
//   - lookup: Typed bool) argument for ResolvePreset.
//
// Returns:
//   - qualityPreset: A stored quality id onto ffmpeg settings.
func ResolvePreset(qualityID string, lookup func(string) (QualityPreset, bool)) QualityPreset {
	if lookup != nil {
		if preset, ok := lookup(qualityID); ok {
			return NormalizePreset(preset)
		}
	}

	if preset, ok := QualityPresets[ClipQuality(qualityID)]; ok {
		return preset
	}

	return QualityPresets[ClipQualityMedium]
}

// ChannelLayoutName names common speaker layouts.
//
// Parameters:
//   - channels: Typed int argument for ChannelLayoutName.
//
// Returns:
//   - value: Result value. Zero or empty when unavailable.
func ChannelLayoutName(channels int) string {
	switch channels {
	case channelsMono:
		return "Mono"
	case channelsStereo:
		return "Stereo"
	case channelsSurround51:
		return "5.1"
	case channelsSurround71:
		return "7.1"
	default:
		if channels <= 0 {
			return ""
		}

		return strconv.Itoa(channels) + "ch"
	}
}
