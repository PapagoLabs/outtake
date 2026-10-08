// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"slices"
	"strconv"
	"strings"
)

// ClipQuality represents a built-in clip quality identifier.
type ClipQuality string

// QualityPreset represents ffmpeg clip encode settings.
type QualityPreset struct {
	CRF       int
	Preset    string
	AudioKbps int
	MaxWidth  int
	// PreserveHDR keeps an HDR source's transfer instead of tone mapping it
	// to SDR. A profile's value is the default for its new video clips.
	PreserveHDR bool
}

const (
	// TransferPQ is ffprobe's HDR10 / PQ transfer name.
	TransferPQ = "smpte2084"
	// TransferHLG is ffprobe's HLG transfer name.
	TransferHLG = "arib-std-b67"
	// TransferPQAlias is a short name some probes use for PQ.
	TransferPQAlias = "pq"
	// TransferHLGAlias is a short name some probes use for HLG.
	TransferHLGAlias = "hlg"
)

const (
	// ClipQualityLow is the low quality preset.
	ClipQualityLow ClipQuality = "low"
	// ClipQualityMedium is the medium quality preset.
	ClipQualityMedium ClipQuality = "medium"
	// ClipQualityHigh is the high quality preset.
	ClipQualityHigh ClipQuality = "high"
	// crfLowQuality is the CRF value for low quality.
	crfLowQuality = 28
	// crfMediumQuality is the CRF value for medium quality.
	crfMediumQuality = 23
	// crfHighQuality is the CRF value for high quality.
	crfHighQuality = 18
	// MinCRF is the lowest allowed CRF.
	MinCRF = 0
	// MaxCRF is the highest allowed CRF. An HEVC encode adds one, capped here.
	MaxCRF = 51
	// MinAudioKbps is the lowest allowed AAC bitrate.
	MinAudioKbps = 64
	// MaxAudioKbps is the highest allowed AAC bitrate.
	MaxAudioKbps = 640
	// audioKbpsLow is the AAC bitrate for the Low profile.
	audioKbpsLow = 128
	// audioKbpsMedium is the AAC bitrate for the Medium profile.
	audioKbpsMedium = 192
	// audioKbpsHigh is the AAC bitrate for the High profile.
	audioKbpsHigh = 320
	// OutputWidth720p is 1280px wide.
	OutputWidth720p = 1280
	// OutputWidth1080p is 1920px wide.
	OutputWidth1080p = 1920
	// OutputWidth1440p is 2560px wide.
	OutputWidth1440p = 2560
	// OutputWidth2160p is 3840px wide (4K).
	OutputWidth2160p = 3840
)

// EncoderPresets lists the -preset values libx264 and libx265 share, from
// fastest to slowest.
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
		CRF:         crfHighQuality,
		Preset:      "slow",
		AudioKbps:   audioKbpsHigh,
		MaxWidth:    OutputWidth2160p,
		PreserveHDR: true,
	},
}

// OutputWidths lists selectable clip export widths.
var OutputWidths = []int{
	OutputWidth720p,
	OutputWidth1080p,
	OutputWidth1440p,
	OutputWidth2160p,
}

// ValidEncoderPreset reports whether name is a supported encoder preset.
//
// Parameters:
//   - name: Candidate preset name.
//
// Returns:
//   - ok: True when the preset is supported.
func ValidEncoderPreset(name string) bool {
	return slices.Contains(EncoderPresets, name)
}

// ValidCRF reports whether crf is in the CRF range.
//
// Parameters:
//   - crf: Candidate constant rate factor.
//
// Returns:
//   - ok: True when the value is in range.
func ValidCRF(crf int) bool {
	return crf >= MinCRF && crf <= MaxCRF
}

// ValidAudioKbps reports whether kbps is in the allowed AAC range.
//
// Parameters:
//   - kbps: Candidate audio bitrate in kilobits per second.
//
// Returns:
//   - ok: True when the bitrate is in range.
func ValidAudioKbps(kbps int) bool {
	return kbps >= MinAudioKbps && kbps <= MaxAudioKbps
}

// NormalizePreset fills missing or invalid encode settings with Medium.
//
// Parameters:
//   - preset: Candidate encode settings.
//
// Returns:
//   - preset: The settings with every invalid field replaced.
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
//   - width: Candidate export width in pixels.
//
// Returns:
//   - ok: True when the width is selectable.
func ValidOutputWidth(width int) bool {
	return slices.Contains(OutputWidths, width)
}

// NormalizeOutputWidth returns width if supported, otherwise 1080p.
//
// Parameters:
//   - width: Candidate export width in pixels.
//
// Returns:
//   - width: The width itself, or OutputWidth1080p.
func NormalizeOutputWidth(width int) int {
	if ValidOutputWidth(width) {
		return width
	}

	return OutputWidth1080p
}

// OutputWidthLabel is the UI label for an export width.
//
// Parameters:
//   - width: Export width in pixels.
//
// Returns:
//   - label: The width as a resolution label, or its raw value when unknown.
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
//   - quality: Stored quality identifier.
//   - lookup: User-defined profile lookup, which may be nil.
//
// Returns:
//   - preset: Encode settings for the quality, normalized.
func ResolvePreset(quality string, lookup func(string) (QualityPreset, bool)) QualityPreset {
	if lookup != nil {
		if preset, ok := lookup(quality); ok {
			return NormalizePreset(preset)
		}
	}

	if preset, ok := QualityPresets[ClipQuality(quality)]; ok {
		return preset
	}

	return QualityPresets[ClipQualityMedium]
}

// IsPQTransfer reports whether an ffprobe color_transfer value is HDR10/PQ.
//
// Parameters:
//   - transfer: ffprobe color_transfer value.
//
// Returns:
//   - ok: True when the transfer is PQ.
func IsPQTransfer(transfer string) bool {
	switch strings.ToLower(strings.TrimSpace(transfer)) {
	case TransferPQ, TransferPQAlias:
		return true
	default:
		return false
	}
}

// IsHLGTransfer reports whether an ffprobe color_transfer value is HLG.
//
// Parameters:
//   - transfer: ffprobe color_transfer value.
//
// Returns:
//   - ok: True when the transfer is HLG.
func IsHLGTransfer(transfer string) bool {
	switch strings.ToLower(strings.TrimSpace(transfer)) {
	case TransferHLG, TransferHLGAlias:
		return true
	default:
		return false
	}
}

// IsHDRTransfer reports whether a stream is HDR, meaning PQ or HLG.
//
// Parameters:
//   - transfer: ffprobe color_transfer value.
//
// Returns:
//   - ok: True when the transfer is PQ or HLG.
func IsHDRTransfer(transfer string) bool {
	return IsPQTransfer(transfer) || IsHLGTransfer(transfer)
}
