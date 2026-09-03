// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"context"
	"slices"
	"strconv"
	"time"
)

// FFmpeg provides FFmpeg media processing capabilities.
type FFmpeg interface {
	Probe(ctx context.Context, path string) (MediaInfo, error)
	ExtractClip(
		ctx context.Context,
		input, output string,
		start, duration float64,
		preset QualityPreset,
		audioIndex int,
		crop CropRect,
	) error
	DetectCrop(ctx context.Context, input string, start, duration float64) (CropRect, error)
	ExtractGIF(
		ctx context.Context,
		input, output string,
		start, duration float64,
		width, fps int,
	) error
	ExtractPreview(
		ctx context.Context,
		input, output string,
		start, duration float64,
		audioIndex int,
		crop CropRect,
	) error
	ExtractScreenshot(ctx context.Context, input, output string, timestamp float64) error
}

// ClipQuality represents a built-in clip quality identifier.
type ClipQuality string

// MediaInfo represents media file information.
type MediaInfo struct {
	Duration    float64      `json:"duration"`
	Width       int          `json:"width"`
	Height      int          `json:"height"`
	VideoCodec  string       `json:"video_codec"`
	AudioCodec  string       `json:"audio_codec"`
	Format      string       `json:"format"`
	BitRate     int64        `json:"bit_rate"`
	AudioTracks []AudioTrack `json:"audio_tracks"`
}

// AudioTrack is one audio stream on a source file.
type AudioTrack struct {
	Index    int
	Codec    string
	Language string
	Title    string
	Channels int
}

// QualityPreset represents ffmpeg clip encode settings.
type QualityPreset struct {
	CRF       int
	Preset    string
	AudioKbps int
	MaxWidth  int
}

const (
	// DefaultFFmpegTimeoutMinutes is the default FFmpeg timeout in minutes.
	defaultFFmpegTimeoutMinutes = 30
	// ClipQualityLow is the low quality preset.
	ClipQualityLow ClipQuality = "low"
	// ClipQualityMedium is the medium quality preset.
	ClipQualityMedium ClipQuality = "medium"
	// ClipQualityHigh is the high quality preset.
	ClipQualityHigh ClipQuality = "high"
	// CrfLowQuality is the CRF value for low quality.
	crfLowQuality = 28
	// CrfMediumQuality is the CRF value for medium quality.
	crfMediumQuality = 23
	// CrfHighQuality is the CRF value for high quality.
	crfHighQuality = 18
	// MinCRF is the lowest allowed libx264 CRF.
	MinCRF = 0
	// MaxCRF is the highest allowed libx264 CRF.
	MaxCRF = 51
	// MinAudioKbps is the lowest allowed AAC bitrate.
	MinAudioKbps = 64
	// MaxAudioKbps is the highest allowed AAC bitrate.
	MaxAudioKbps = 640
	// AudioKbpsLow is the AAC bitrate for the Low profile.
	audioKbpsLow = 128
	// AudioKbpsMedium is the AAC bitrate for the Medium profile.
	audioKbpsMedium = 192
	// AudioKbpsHigh is the AAC bitrate for the High profile.
	audioKbpsHigh = 320
	// ChannelsMono is a single-channel layout.
	channelsMono = 1
	// ChannelsStereo is a two-channel layout.
	channelsStereo = 2
	// ChannelsSurround51 is a 5.1 layout.
	channelsSurround51 = 6
	// ChannelsSurround71 is a 7.1 layout.
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

// DefaultFFmpegTimeout returns the default FFmpeg timeout.
func DefaultFFmpegTimeout() time.Duration {
	return defaultFFmpegTimeoutMinutes * time.Minute
}

// ValidEncoderPreset reports whether name is a supported libx264 preset.
func ValidEncoderPreset(name string) bool {
	return slices.Contains(EncoderPresets, name)
}

// ValidCRF reports whether crf is in the libx264 range.
func ValidCRF(crf int) bool {
	return crf >= MinCRF && crf <= MaxCRF
}

// ValidAudioKbps reports whether kbps is in the allowed AAC range.
func ValidAudioKbps(kbps int) bool {
	return kbps >= MinAudioKbps && kbps <= MaxAudioKbps
}

// NormalizePreset fills missing or invalid encode settings with Medium.
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
func ValidOutputWidth(width int) bool {
	return slices.Contains(OutputWidths, width)
}

// NormalizeOutputWidth returns width if supported, otherwise 1080p.
func NormalizeOutputWidth(width int) int {
	if ValidOutputWidth(width) {
		return width
	}

	return OutputWidth1080p
}

// OutputWidthLabel is the UI label for an export width.
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
// Lookup is tried first so user-defined profiles win over built-in names.
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

// ChannelLayoutName names common speaker layouts.
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
