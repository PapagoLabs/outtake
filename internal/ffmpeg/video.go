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

// videoEncodeRequest is the input for a clip or preview video encode.
type videoEncodeRequest struct {
	ffmpegPath string
	input      string
	output     string
	start      time.Duration
	duration   time.Duration
	preset     clip.QualityPreset
	audioIndex int
	maxWidth   int
	scaleFlags string
	crop       crop.CropRect
	hdrKind    string
	toneMap    bool
	colorTags  []string
	pixFmt     string
	encoder    string
	tonePeak   float64
}

const (
	// videoCodecH264 encodes SDR and tone mapped video.
	videoCodecH264 = "libx264"
	// videoCodecHEVC encodes video that keeps HDR.
	videoCodecHEVC = "libx265"
	// hevcProfile is the 10-bit HEVC profile kept HDR is encoded in.
	hevcProfile = "main10"
	// hevcTag is the MP4 sample entry Apple players and browsers require for
	// HEVC. FFmpeg 8 and later move the parameter sets out of its samples.
	hevcTag = "hvc1"
	// hevcCRFOffset is how much higher an x265 CRF is than the x264 CRF of
	// the same visual quality.
	hevcCRFOffset = 1
	// defaultAudioCodec is the default audio codec.
	defaultAudioCodec = "aac"
	// scaleFlagsLanczos is the scaler used for saved clip scaling.
	scaleFlagsLanczos = "lanczos"
	// scaleFlagsFast is the scaler used for preview scaling.
	scaleFlagsFast = "fast_bilinear"
	// previewMaxWidth is the widest a preview is encoded when its request
	// names no maximum.
	previewMaxWidth = clip.OutputWidth1080p
	// previewCRF is the libx264 CRF for previews.
	previewCRF = 30
	// previewPreset is the libx264 preset for previews.
	previewPreset = "ultrafast"
	// previewAudioKbps is the AAC bitrate for previews.
	previewAudioKbps = 96
	// overwriteFlag is the movflags value that relocates the MP4 index to the front.
	overwriteFlag = "+faststart"
	// encodeClipErrFmt is the message clip encode failures are wrapped with.
	encodeClipErrFmt = "encode clip: %w"
	// encodePreviewErrFmt is the message preview encode failures are wrapped with.
	encodePreviewErrFmt = "encode preview: %w"
)

// ExtractClip encodes a clip segment, with x265 when it keeps HDR and x264
// otherwise.
//
// Parameters:
//   - ctx: Cancellation and deadline for the encode.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset into the source.
//   - duration: Length of the clip.
//   - preset: Encode quality, including whether HDR is kept.
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
		return fmt.Errorf(encodeClipErrFmt, err)
	}

	cleanOutput, err := outputPath(output)
	if err != nil {
		return fmt.Errorf(encodeClipErrFmt, err)
	}

	err = publish(ctx, cleanOutput, func(staging string) error {
		req := clipEncodeRequest(
			execFFmpeg.ffmpegPath,
			cleanInput,
			staging,
			start,
			duration,
			preset,
			audioIndex,
			rect,
		)
		execFFmpeg.resolveColor(ctx, &req)

		return execFFmpeg.run(ctx, duration, videoEncodeArgs(&req)...)
	})
	if err != nil {
		return fmt.Errorf(encodeClipErrFmt, err)
	}

	return nil
}

// clipEncodeRequest builds the shared encode request for a saved clip.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset into the source.
//   - duration: Length of the clip.
//   - preset: Encode quality, including whether HDR is kept.
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
) videoEncodeRequest {
	// The transfer is resolved from the source before the arguments are built,
	// so it is not seeded here.
	hdrKind := ""

	return videoEncodeRequest{
		ffmpegPath: ffmpegPath,
		input:      input,
		output:     output,
		start:      start,
		duration:   duration,
		preset:     preset,
		audioIndex: audioIndex,
		maxWidth:   clip.NormalizeOutputWidth(preset.MaxWidth),
		scaleFlags: scaleFlagsLanczos,
		crop:       rect,
		hdrKind:    hdrKind,
		tonePeak:   tonemap.DefaultWebSafePeak,
	}
}

// previewEncodeRequest builds the shared encode request for a preview.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset into the source.
//   - duration: Length of the preview window.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//   - preset: Encode options; only PreserveHDR and MaxWidth are read. A
//     narrower source keeps its width, and a zero MaxWidth caps at 1080p.
//
// Returns:
//   - req: Populated encode request. HDR peak is filled later by resolveColor.
func previewEncodeRequest(
	ffmpegPath, input, output string,
	start, duration time.Duration,
	audioIndex int,
	rect crop.CropRect,
	preset clip.QualityPreset,
) videoEncodeRequest {
	// The transfer is resolved from the source before the arguments are built,
	// so it is not seeded here.
	hdrKind := ""

	maxWidth := preset.MaxWidth
	if maxWidth <= 0 {
		maxWidth = previewMaxWidth
	}

	return videoEncodeRequest{
		ffmpegPath: ffmpegPath,
		input:      input,
		output:     output,
		start:      start,
		duration:   duration,
		preset: clip.QualityPreset{
			CRF:         previewCRF,
			Preset:      previewPreset,
			AudioKbps:   previewAudioKbps,
			MaxWidth:    maxWidth,
			PreserveHDR: preset.PreserveHDR,
		},
		audioIndex: audioIndex,
		maxWidth:   maxWidth,
		scaleFlags: scaleFlagsFast,
		crop:       rect,
		hdrKind:    hdrKind,
		tonePeak:   tonemap.DefaultWebSafePeak,
	}
}

// ExtractPreview writes a short, downscaled preview segment, encoded like the
// clip it previews.
//
// Parameters:
//   - ctx: Cancellation and deadline for the encode.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset into the source.
//   - duration: Requested window, capped by PreviewDuration.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//   - preset: Encode options; only PreserveHDR and MaxWidth are read. A
//     narrower source keeps its width, and a zero MaxWidth caps at 1080p.
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
		return fmt.Errorf(encodePreviewErrFmt, err)
	}

	cleanOutput, err := outputPath(output)
	if err != nil {
		return fmt.Errorf(encodePreviewErrFmt, err)
	}

	duration = PreviewDuration(duration)

	runErr := publish(ctx, cleanOutput, func(staging string) error {
		req := previewEncodeRequest(
			execFFmpeg.ffmpegPath,
			cleanInput,
			staging,
			start,
			duration,
			audioIndex,
			rect,
			preset,
		)
		execFFmpeg.resolveColor(ctx, &req)

		return execFFmpeg.run(ctx, duration, videoEncodeArgs(&req)...)
	})
	if runErr != nil {
		return fmt.Errorf(encodePreviewErrFmt, runErr)
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
func pixelFormat(req *videoEncodeRequest) string {
	if req.pixFmt != "" {
		return req.pixFmt
	}

	return pixelFormatYUV420P
}

// videoFilter applies optional black-bar crop, optional HDR tone-map, then scale.
//
// Parameters:
//   - req: Encode request with crop, scale, and tone map fields.
//
// Returns:
//   - filter: The ffmpeg -vf chain.
func videoFilter(req *videoEncodeRequest) string {
	chain := scaleFilter(req.maxWidth, req.scaleFlags)
	if req.toneMap && req.hdrKind != "" {
		chain = tonemap.ToneMapFilter(
			req.hdrKind,
			req.tonePeak,
			tonemap.TransferBT709,
		) + "," + chain
	}

	return prependCrop(req.crop, chain)
}

// videoEncodeArgs builds the argv of a clip or preview encode.
//
// Parameters:
//   - req: Encode request for a clip or preview.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func videoEncodeArgs(req *videoEncodeRequest) []string {
	preset := clip.NormalizePreset(req.preset)
	audioIndex := max(req.audioIndex, 0)

	args := []string{
		req.ffmpegPath,
		outputFlag,
		abortOnFlag, abortOnEmptyOutput,
		ssFlag, timecode.FromDuration(req.start).FormatSeconds(),
		inputFlag, req.input,
		durationFlag, timecode.FromDuration(req.duration).FormatSeconds(),
		"-map", "0:v:0",
		"-map", "0:a:" + strconv.Itoa(audioIndex) + "?",
	}

	args = append(args, codecArgs(req.encoder, preset)...)
	args = append(args,
		pixelFormatFlag, pixelFormat(req),
		videoFilterFlag, videoFilter(req),
		"-c:a", defaultAudioCodec,
		"-b:a", strconv.Itoa(preset.AudioKbps)+"k",
		"-ac", "2",
	)

	args = append(args, req.colorTags...)

	// The source's title, tags, and chapters describe the whole source, not
	// the clip, so none of them is copied into it.
	args = append(args, mapMetadataFlag, dropAll, mapChaptersFlag, dropAll)

	return append(args, "-movflags", movFlags(req), req.output)
}

// codecArgs selects the video encoder and its quality. HEVC is encoded as
// 10-bit Main 10 in an hvc1 track, at the x264 CRF plus hevcCRFOffset.
//
// Parameters:
//   - encoder: The planned encoder, empty for H.264.
//   - preset: Normalized encode settings.
//
// Returns:
//   - args: The encoder, profile, tag, CRF, and preset flags.
func codecArgs(encoder string, preset clip.QualityPreset) []string {
	if encoder != videoCodecHEVC {
		return []string{
			"-c:v", videoCodecH264,
			"-crf", strconv.Itoa(preset.CRF),
			"-preset", preset.Preset,
		}
	}

	return []string{
		"-c:v", videoCodecHEVC,
		"-profile:v", hevcProfile,
		"-tag:v", hevcTag,
		"-crf", strconv.Itoa(min(preset.CRF+hevcCRFOffset, clip.MaxCRF)),
		"-preset", preset.Preset,
	}
}

// movFlags returns container flags, adding colr when the output was retagged.
//
// Parameters:
//   - req: Encode request whose tone-map decision selects the movflags value.
//
// Returns:
//   - flags: +faststart, or +faststart+write_colr once the tone map retagged the
//     output as Rec.709.
func movFlags(req *videoEncodeRequest) string {
	if req.toneMap {
		return toneMappedMovFlags
	}

	return overwriteFlag
}
