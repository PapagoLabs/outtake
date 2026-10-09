// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"slices"
	"strconv"
	"strings"
)

// QualityPreset represents ffmpeg clip encode settings.
type QualityPreset struct {
	CRF       int
	Preset    string
	AudioKbps int
	MaxWidth  int
	// PreserveHDR keeps an HDR source's transfer instead of tone mapping it
	// to SDR. A clip takes its profile's value each time it renders.
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
	// defaultCRF is the 1080p profile's CRF.
	defaultCRF = 20
	// presetMedium is the medium encoder preset, which the 1080p profile uses.
	presetMedium = "medium"
	// defaultAudioKbps is the 1080p profile's AAC bitrate.
	defaultAudioKbps = 192
	// MinCRF is the lowest allowed CRF.
	MinCRF = 0
	// MaxCRF is the highest allowed CRF. An HEVC encode adds one, capped here.
	MaxCRF = 51
	// MinAudioKbps is the lowest allowed AAC bitrate.
	MinAudioKbps = 64
	// MaxAudioKbps is the highest allowed AAC bitrate.
	MaxAudioKbps = 640
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
	presetMedium,
	"slow",
	"slower",
	"veryslow",
}

// DefaultPreset is the 1080p profile's settings, which a render uses when no
// stored profile answers: with no database, or for a clip whose profile is
// gone. The stored profiles, including the built-ins, live in clip_profiles.
var DefaultPreset = QualityPreset{
	CRF:         defaultCRF,
	Preset:      presetMedium,
	AudioKbps:   defaultAudioKbps,
	MaxWidth:    OutputWidth1080p,
	PreserveHDR: false,
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

// NormalizePreset fills missing or invalid encode settings from DefaultPreset.
//
// Parameters:
//   - preset: Candidate encode settings.
//
// Returns:
//   - preset: The settings with every invalid field replaced.
func NormalizePreset(preset QualityPreset) QualityPreset {
	if !ValidCRF(preset.CRF) || !ValidEncoderPreset(preset.Preset) {
		return DefaultPreset
	}

	if !ValidAudioKbps(preset.AudioKbps) {
		preset.AudioKbps = DefaultPreset.AudioKbps
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
//   - lookup: Stored profile lookup, which may be nil.
//
// Returns:
//   - preset: Encode settings for the quality, normalized, or DefaultPreset
//     when no stored profile has the id.
func ResolvePreset(quality string, lookup func(string) (QualityPreset, bool)) QualityPreset {
	if lookup != nil {
		if preset, ok := lookup(quality); ok {
			return NormalizePreset(preset)
		}
	}

	return DefaultPreset
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
