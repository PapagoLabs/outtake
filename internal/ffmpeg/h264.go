// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/tonemap"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

// h264EncodeRequest is the input for a browser-safe libx264 encode.
type h264EncodeRequest struct {
	ffmpegPath   string
	input        string
	output       string
	start        time.Duration
	duration     time.Duration
	preset       clip.QualityPreset
	audioIndex   int
	maxWidth     int
	scaleFlags   string
	crop         crop.CropRect
	webSafeColor bool
	hdrKind      string
	toneMap      bool
	colorTags    []string
	pixFmt       string
	tonePeak     float64
}

const (
	// defaultVideoCodec is the default video codec.
	defaultVideoCodec = "libx264"
	// defaultAudioCodec is the default audio codec.
	defaultAudioCodec = "aac"
	// scaleFlagsLanczos is the scaler used for saved clip scaling.
	scaleFlagsLanczos = "lanczos"
	// scaleFlagsFast is the scaler used for preview scaling.
	scaleFlagsFast = "fast_bilinear"
	// previewMaxWidth is the maximum width for preview encodes.
	previewMaxWidth = 1280
	// previewCRF is the libx264 CRF for previews.
	previewCRF = 30
	// previewPreset is the libx264 preset for previews.
	previewPreset = "ultrafast"
	// previewAudioKbps is the AAC bitrate for previews.
	previewAudioKbps = 96
	// overwriteFlag is the movflags value that relocates the MP4 index to the front.
	overwriteFlag = "+faststart"
)

// ExtractClip encodes a clip segment with x264.
//
// Parameters:
//   - ctx: Cancellation and deadline for the encode.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset into the source.
//   - duration: Length of the clip.
//   - preset: Encode quality, including WebSafeColor.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - err: Non-nil when the clip could not be encoded.
func (execFFmpeg *ExecFFmpeg) ExtractClip(
	ctx context.Context,
	input, output string,
	start, duration time.Duration,
	preset clip.QualityPreset,
	audioIndex int,
	rect crop.CropRect,
) error {
	// Build and run the clip ffmpeg command.
	cleanInput, err := mediaPath(input)
	if err != nil {
		return fmt.Errorf("encode clip: %w", err)
	}

	cleanOutput, err := mediaPath(output)
	if err != nil {
		return fmt.Errorf("encode clip: %w", err)
	}

	req := clipEncodeRequest(
		execFFmpeg.ffmpegPath,
		cleanInput,
		cleanOutput,
		start,
		duration,
		preset,
		audioIndex,
		rect,
	)
	execFFmpeg.resolveColor(ctx, &req)

	err = execFFmpeg.run(ctx, duration, h264EncodeArgs(&req)...)
	if err != nil {
		return fmt.Errorf("encode clip: %w", err)
	}

	return nil
}

// clipEncodeRequest builds the shared H.264 encode request for a saved clip.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset into the source.
//   - duration: Length of the clip.
//   - preset: Encode quality, including WebSafeColor.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - req: Populated encode request. HDR peak is filled later by resolveColor.
func clipEncodeRequest(
	ffmpegPath, input, output string,
	start, duration time.Duration,
	preset clip.QualityPreset,
	audioIndex int,
	rect crop.CropRect,
) h264EncodeRequest {
	// The transfer is resolved from the source before the arguments are built,
	// so it is not seeded here.
	hdrKind := ""

	return h264EncodeRequest{
		ffmpegPath:   ffmpegPath,
		input:        input,
		output:       output,
		start:        start,
		duration:     duration,
		preset:       preset,
		audioIndex:   audioIndex,
		maxWidth:     clip.NormalizeOutputWidth(preset.MaxWidth),
		scaleFlags:   scaleFlagsLanczos,
		crop:         rect,
		webSafeColor: preset.WebSafeColor,
		hdrKind:      hdrKind,
		tonePeak:     tonemap.DefaultWebSafePeak,
	}
}

// previewEncodeRequest builds the shared H.264 encode request for a preview.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset into the source.
//   - duration: Length of the preview window.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//   - preset: Encode options; only WebSafeColor is read.
//
// Returns:
//   - req: Populated encode request. HDR peak is filled later by resolveColor.
func previewEncodeRequest(
	ffmpegPath, input, output string,
	start, duration time.Duration,
	audioIndex int,
	rect crop.CropRect,
	preset clip.QualityPreset,
) h264EncodeRequest {
	// The transfer is resolved from the source before the arguments are built,
	// so it is not seeded here.
	hdrKind := ""

	return h264EncodeRequest{
		ffmpegPath: ffmpegPath,
		input:      input,
		output:     output,
		start:      start,
		duration:   duration,
		preset: clip.QualityPreset{
			CRF:          previewCRF,
			Preset:       previewPreset,
			AudioKbps:    previewAudioKbps,
			MaxWidth:     previewMaxWidth,
			WebSafeColor: preset.WebSafeColor,
			PreserveHDR:  preset.PreserveHDR,
		},
		audioIndex:   audioIndex,
		maxWidth:     previewMaxWidth,
		scaleFlags:   scaleFlagsFast,
		crop:         rect,
		webSafeColor: preset.WebSafeColor,
		hdrKind:      hdrKind,
		tonePeak:     tonemap.DefaultWebSafePeak,
	}
}

// ExtractPreview writes a short, downscaled, browser-safe preview segment.
//
// Parameters:
//   - ctx: Cancellation and deadline for the encode.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset into the source.
//   - duration: Requested window, capped by PreviewDuration.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//   - preset: Encode options; only WebSafeColor is read.
//
// Returns:
//   - err: Non-nil when the preview could not be encoded.
func (execFFmpeg *ExecFFmpeg) ExtractPreview(
	ctx context.Context,
	input, output string,
	start, duration time.Duration,
	audioIndex int,
	rect crop.CropRect,
	preset clip.QualityPreset,
) error {
	cleanInput, err := mediaPath(input)
	if err != nil {
		return fmt.Errorf("encode preview: %w", err)
	}

	cleanOutput, err := mediaPath(output)
	if err != nil {
		return fmt.Errorf("encode preview: %w", err)
	}

	duration = PreviewDuration(duration)

	req := previewEncodeRequest(
		execFFmpeg.ffmpegPath,
		cleanInput,
		cleanOutput,
		start,
		duration,
		audioIndex,
		rect,
		preset,
	)
	execFFmpeg.resolveColor(ctx, &req)

	runErr := execFFmpeg.run(ctx, duration, h264EncodeArgs(&req)...)
	if runErr != nil {
		return fmt.Errorf("encode preview: %w", runErr)
	}

	return nil
}

// PreviewDuration caps a preview window so encodes stay cheap.
//
// Parameters:
//   - duration: Requested clip duration.
//
// Returns:
//   - capped: The window that will actually be encoded.
func PreviewDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return 0
	}

	return min(duration, clip.MaxDuration)
}

// scaleFilter downscales to maxWidth while keeping even dimensions.
//
// Parameters:
//   - maxWidth: Upper bound on the output width in pixels.
//   - flags: Scaler flags, such as lanczos.
//
// Returns:
//   - filter: The ffmpeg scale filter string.
func scaleFilter(maxWidth int, flags string) string {
	return fmt.Sprintf("scale=w='trunc(min(%d,iw)/2)*2':h=-2:flags=%s", maxWidth, flags)
}

// pixelFormat reports the pixel format an encode should carry.
//
// Parameters:
//   - req: Encode request.
//
// Returns:
//   - pixFmt: The requested format, or 8-bit 4:2:0 when none was planned.
func pixelFormat(req *h264EncodeRequest) string {
	if req.pixFmt != "" {
		return req.pixFmt
	}

	return pixelFormatYUV420P
}

// videoFilter applies optional black-bar crop, optional HDR tone-map, then scale.
//
// Parameters:
//   - req: Encode request with crop, scale, and web-safe color fields.
//
// Returns:
//   - filter: The ffmpeg -vf chain.
func videoFilter(req *h264EncodeRequest) string {
	chain := scaleFilter(req.maxWidth, req.scaleFlags)
	if req.toneMap && req.hdrKind != "" {
		chain = tonemap.ToneMapFilter(req.hdrKind, req.tonePeak) + "," + chain
	}

	return prependCrop(req.crop, chain)
}

// h264EncodeArgs builds a browser-safe libx264 argv.
//
// Parameters:
//   - req: Encode request for a clip or preview.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func h264EncodeArgs(req *h264EncodeRequest) []string {
	preset := clip.NormalizePreset(req.preset)
	audioIndex := max(req.audioIndex, 0)

	args := []string{
		req.ffmpegPath,
		outputFlag,
		ssFlag, timecode.FromDuration(req.start).FormatSeconds(),
		inputFlag, req.input,
		durationFlag, timecode.FromDuration(req.duration).FormatSeconds(),
		"-map", "0:v:0",
		"-map", "0:a:" + strconv.Itoa(audioIndex) + "?",
		"-c:v", defaultVideoCodec,
		pixelFormatFlag, pixelFormat(req),
		videoFilterFlag, videoFilter(req),
		"-crf", strconv.Itoa(preset.CRF),
		"-preset", preset.Preset,
		"-c:a", defaultAudioCodec,
		"-b:a", strconv.Itoa(preset.AudioKbps) + "k",
		"-ac", "2",
	}

	args = append(args, req.colorTags...)

	return append(args, "-movflags", movFlags(req), req.output)
}

// movFlags returns container flags, adding colr when the output was retagged.
//
// Parameters:
//   - req: Encode request whose tone-map decision selects the movflags value.
//
// Returns:
//   - flags: +faststart, or +faststart+write_colr once the tone map retagged the
//     output as Rec.709.
func movFlags(req *h264EncodeRequest) string {
	if req.toneMap {
		return webSafeMovFlags
	}

	return overwriteFlag
}
