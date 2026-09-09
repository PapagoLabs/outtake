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
		crop CropRect,
	) error
	ExtractPreview(
		ctx context.Context,
		input, output string,
		start, duration float64,
		audioIndex int,
		crop CropRect,
		preset QualityPreset,
	) error
	ExtractScreenshot(
		ctx context.Context,
		input, output string,
		timestamp float64,
		crop CropRect,
	) error
}

const defaultFFmpegTimeoutMinutes = 30

// DefaultFFmpegTimeout returns the default FFmpeg timeout.
//
// Returns:
//   - dur: The default FFmpeg timeout.
func DefaultFFmpegTimeout() time.Duration {
	return defaultFFmpegTimeoutMinutes * time.Minute
}
