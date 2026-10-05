// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"strconv"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
)

const (
	// channelsMono is a single-channel layout.
	channelsMono = 1
	// channelsStereo is a two-channel layout.
	channelsStereo = 2
	// channelsSurround51 is a 5.1 layout.
	channelsSurround51 = 6
	// channelsSurround71 is a 7.1 layout.
	channelsSurround71 = 8
	// Height720p is the smallest height still called 720p.
	Height720p = 720
	// Height1080p is the smallest height still called 1080p.
	Height1080p = 1080
	// Height2160p is the smallest height called 4K.
	Height2160p = 2160
)

// SourceQuality is a short label for what a source's video is.
//
// Parameters:
//   - info: Probed information for the source.
//
// Returns:
//   - label: The resolution and dynamic range, or empty when neither is known.
func SourceQuality(info probe.Info) string {
	resolution := SourceResolutionLabel(info.Height)
	transfer := DynamicRangeLabel(info.ColorTransfer)

	switch {
	case resolution == "" && transfer == "":
		return ""
	case resolution == "":
		return transfer
	case transfer == "":
		return resolution
	default:
		return resolution + " " + transfer
	}
}

// SourceResolutionLabel classifies a source height the way a viewer would name
// it.
//
// Parameters:
//   - height: Video height in pixels.
//
// Returns:
//   - label: The class name, or empty when the height is unknown.
func SourceResolutionLabel(height int) string {
	switch {
	case height >= Height2160p:
		return "4K"
	case height >= Height1080p:
		return "1080p"
	case height >= Height720p:
		return "720p"
	case height > 0:
		return strconv.Itoa(height) + "p"
	default:
		return ""
	}
}

// DynamicRangeLabel names a color transfer for display.
//
// Parameters:
//   - transfer: ffprobe color_transfer of the source.
//
// Returns:
//   - label: The transfer's display name, or empty when it is not HDR.
func DynamicRangeLabel(transfer string) string {
	switch {
	case clip.IsPQTransfer(transfer):
		return "HDR10"
	case clip.IsHLGTransfer(transfer):
		return "HLG"
	default:
		return ""
	}
}

// IsHDRSource reports whether a source carries an HDR transfer.
//
// Parameters:
//   - transfer: ffprobe color_transfer of the source.
//
// Returns:
//   - hdr: True when the source is PQ or HLG.
func IsHDRSource(transfer string) bool {
	return clip.IsHDRTransfer(transfer)
}

// ChannelLayoutName names common speaker layouts.
//
// Parameters:
//   - channels: Audio channel count on the source.
//
// Returns:
//   - name: The layout's display name, or empty when the count is unknown.
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
