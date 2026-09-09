// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"github.com/PapagoLabs/outtake/internal/media/quality"
)

// ClipQuality represents a built-in clip quality identifier.
type ClipQuality = quality.ClipQuality

// QualityPreset represents ffmpeg clip encode settings.
type QualityPreset = quality.QualityPreset

const (
	// ClipQualityLow is the low quality preset.
	ClipQualityLow = quality.ClipQualityLow
	// ClipQualityMedium is the medium quality preset.
	ClipQualityMedium = quality.ClipQualityMedium
	// ClipQualityHigh is the high quality preset.
	ClipQualityHigh = quality.ClipQualityHigh
	// MinCRF is the lowest allowed libx264 CRF.
	MinCRF = quality.MinCRF
	// MaxCRF is the highest allowed libx264 CRF.
	MaxCRF = quality.MaxCRF
	// MinAudioKbps is the lowest allowed AAC bitrate.
	MinAudioKbps = quality.MinAudioKbps
	// MaxAudioKbps is the highest allowed AAC bitrate.
	MaxAudioKbps = quality.MaxAudioKbps
	// OutputWidth720p is 1280px wide.
	OutputWidth720p = quality.OutputWidth720p
	// OutputWidth1080p is 1920px wide.
	OutputWidth1080p = quality.OutputWidth1080p
	// OutputWidth1440p is 2560px wide.
	OutputWidth1440p = quality.OutputWidth1440p
	// OutputWidth2160p is 3840px wide (4K).
	OutputWidth2160p = quality.OutputWidth2160p
)

// EncoderPresets lists valid libx264 -preset values from fastest to slowest.
var EncoderPresets = quality.EncoderPresets

// QualityPresets maps built-in quality identifiers to presets.
var QualityPresets = quality.QualityPresets

// OutputWidths lists selectable clip export widths.
var OutputWidths = quality.OutputWidths

// ValidEncoderPreset reports whether name is a supported libx264 preset.
func ValidEncoderPreset(name string) bool {
	return quality.ValidEncoderPreset(name)
}

// ValidCRF reports whether crf is in the libx264 range.
func ValidCRF(crf int) bool {
	return quality.ValidCRF(crf)
}

// ValidAudioKbps reports whether kbps is in the allowed AAC range.
func ValidAudioKbps(kbps int) bool {
	return quality.ValidAudioKbps(kbps)
}

// NormalizePreset fills missing or invalid encode settings with Medium.
func NormalizePreset(preset QualityPreset) QualityPreset {
	return quality.NormalizePreset(preset)
}

// ValidOutputWidth reports whether width is a supported export width.
func ValidOutputWidth(width int) bool {
	return quality.ValidOutputWidth(width)
}

// NormalizeOutputWidth returns width if supported, otherwise 1080p.
func NormalizeOutputWidth(width int) int {
	return quality.NormalizeOutputWidth(width)
}

// OutputWidthLabel is the UI label for an export width.
func OutputWidthLabel(width int) string {
	return quality.OutputWidthLabel(width)
}

// ResolvePreset maps a stored quality id onto ffmpeg settings.
func ResolvePreset(qualityID string, lookup func(string) (QualityPreset, bool)) QualityPreset {
	return quality.ResolvePreset(qualityID, lookup)
}

// ChannelLayoutName names common speaker layouts.
func ChannelLayoutName(channels int) string {
	return quality.ChannelLayoutName(channels)
}
