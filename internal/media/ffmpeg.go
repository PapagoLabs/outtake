// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"context"
	"time"
)

// FFmpeg provides FFmpeg media processing capabilities.
type FFmpeg interface {
	Probe(ctx context.Context, path string) (MediaInfo, error)
	ExtractClip(
		ctx context.Context,
		input, output string,
		start, duration float64,
		quality ClipQuality,
	) error
	DetectCrop(ctx context.Context, input string, start, duration float64) (CropRect, error)
	ExtractGIF(
		ctx context.Context,
		input, output string,
		start, duration float64,
		width, fps int,
	) error
	ExtractScreenshot(ctx context.Context, input, output string, timestamp float64) error
}

// ClipQuality represents the quality of a clip.
type ClipQuality string

// MediaInfo represents media file information.
type MediaInfo struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	VideoCodec string  `json:"video_codec"`
	AudioCodec string  `json:"audio_codec"`
	Format     string  `json:"format"`
	BitRate    int64   `json:"bit_rate"`
}

// QualityPreset represents a quality preset.
type QualityPreset struct {
	CRF    int
	Preset string
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
)

// QualityPresets maps quality levels to presets.
var QualityPresets = map[ClipQuality]QualityPreset{
	ClipQualityLow:    {CRF: crfLowQuality, Preset: "veryfast"},
	ClipQualityMedium: {CRF: crfMediumQuality, Preset: "medium"},
	ClipQualityHigh:   {CRF: crfHighQuality, Preset: "slow"},
}

// DefaultFFmpegTimeout returns the default FFmpeg timeout.
func DefaultFFmpegTimeout() time.Duration {
	return defaultFFmpegTimeoutMinutes * time.Minute
}
