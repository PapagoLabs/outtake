// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"context"
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

const (
	// defaultFPS is the default frames per second.
	defaultFPS = 10
	// defaultWidth is the default width.
	defaultWidth = 480
	// updateFlag tells image2 to overwrite a single still.
	updateFlag = "-update"
	// updateEnabled is the image2 single-file update value.
	updateEnabled = "1"
	// filterComplexFlag is the FFmpeg filter_complex flag.
	filterComplexFlag = "-filter_complex"
	// gifScaleHeight scales GIF height to an even value.
	gifScaleHeight = "-2"
	// encodeGIFErrFmt is the message GIF encode failures are wrapped with.
	encodeGIFErrFmt = "encode gif: %w"
)

// gifScaleFilter is the shared fps+scale chain for GIF palette and encode.
//
// Parameters:
//   - width: Output width in pixels.
//   - fps: Output frames per second.
//
// Returns:
//   - filter: fps and even-height scale fragment.
func gifScaleFilter(width, fps int) string {
	// Width is forced even. format=yuv420p rejects an odd width, and the form
	// accepts any width from 120 through 1920.
	return fmt.Sprintf(
		"fps=%d,scale='trunc(%d/2)*2':%s:flags=lanczos",
		fps,
		width,
		gifScaleHeight,
	)
}

// gifVideoChain is the shared decode/scale chain for both GIF passes.
//
// Parameters:
//   - width: Output width in pixels.
//   - fps: Output frames per second.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - filter: crop, fps, scale, and yuv420p conversion.
func gifVideoChain(width, fps int, rect crop.CropRect) string {
	return prependCrop(rect, gifScaleFilter(width, fps)+",format=yuv420p")
}

// gifPaletteFilter builds the palettegen -vf chain, with optional crop first.
//
// Parameters:
//   - width: Output width in pixels.
//   - fps: Output frames per second.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - filter: Palette generation -vf string.
func gifPaletteFilter(width, fps int, rect crop.CropRect) string {
	return gifVideoChain(width, fps, rect) + ",palettegen=stats_mode=diff"
}

// gifEncodeFilter builds the paletteuse -filter_complex chain.
//
// Parameters:
//   - width: Output width in pixels.
//   - fps: Output frames per second.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - filter: Labeled paletteuse filter_complex string.
func gifEncodeFilter(width, fps int, rect crop.CropRect) string {
	return "[0:v]" + gifVideoChain(width, fps, rect) +
		"[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5"
}

// gifSeekArgs prefixes ffmpeg with an input-limited seek.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - start: Seek offset into the source.
//   - duration: Length of the input window.
//
// Returns:
//   - args: argv through the first -i inclusive.
func gifSeekArgs(ffmpegPath, input string, start, duration time.Duration) []string {
	return []string{
		ffmpegPath,
		outputFlag,
		ssFlag,
		timecode.FromDuration(start).FormatSeconds(),
		durationFlag,
		timecode.FromDuration(duration).FormatSeconds(),
		inputFlag,
		input,
	}
}

// gifPaletteArgs builds the ffmpeg palettegen command.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - palette: Destination palette PNG path.
//   - start: Seek offset into the source.
//   - duration: Length of the input window.
//   - vf: palettegen -vf chain.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func gifPaletteArgs(
	ffmpegPath, input, palette string,
	start, duration time.Duration,
	vf string,
) []string {
	args := gifSeekArgs(ffmpegPath, input, start, duration)

	args = append(
		args,
		anFlag,
		videoFilterFlag,
		vf,
		framesFlag,
		"1",
		updateFlag,
		updateEnabled,
		palette,
	)

	return args
}

// gifEncodeArgs builds the ffmpeg paletteuse command.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - palette: Palette PNG path.
//   - output: Destination GIF path.
//   - start: Seek offset into the source.
//   - duration: Length of the input window.
//   - filter: paletteuse filter_complex string.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func gifEncodeArgs(
	ffmpegPath, input, palette, output string,
	start, duration time.Duration,
	filter string,
) []string {
	args := gifSeekArgs(ffmpegPath, input, start, duration)

	args = append(args, inputFlag, palette, anFlag, filterComplexFlag, filter, output)

	return args
}

// ExtractGIF extracts a GIF from a video using the palettegen/paletteuse pair.
//
// Parameters:
//   - ctx: Cancellation and deadline for both passes.
//   - input: Source media path.
//   - output: Destination GIF path.
//   - start: Seek offset into the source.
//   - duration: Length of the GIF window.
//   - width: Output width in pixels, or 0 for the default.
//   - fps: Output frames per second, or 0 for the default.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - err: Non-nil when either pass failed.
func (execFFmpeg *ExecFFmpeg) ExtractGIF(
	ctx context.Context,
	input, output string,
	start, duration time.Duration,
	width, fps int,
	rect crop.CropRect,
) error {
	// Build and run the two-pass GIF ffmpeg command.
	cleanInput, err := mediaPath(input)
	if err != nil {
		return fmt.Errorf(encodeGIFErrFmt, err)
	}

	cleanOutput, err := mediaPath(output)
	if err != nil {
		return fmt.Errorf(encodeGIFErrFmt, err)
	}

	if width <= 0 {
		width = defaultWidth
	}
	if fps <= 0 {
		fps = defaultFPS
	}

	palettePath := cleanOutput + ".palette.png"
	// palettegen can create the PNG and then fail. The remove has to be armed
	// before that run, or the file is left next to the output.
	defer osRemove(palettePath)

	err = execFFmpeg.run(
		ctx,
		duration,
		gifPaletteArgs(
			execFFmpeg.ffmpegPath,
			cleanInput,
			palettePath,
			start,
			duration,
			gifPaletteFilter(width, fps, rect),
		)...,
	)
	if err != nil {
		return fmt.Errorf("palettegen: %w", err)
	}

	gifArgs := gifEncodeArgs(
		execFFmpeg.ffmpegPath,
		cleanInput,
		palettePath,
		cleanOutput,
		start,
		duration,
		gifEncodeFilter(width, fps, rect),
	)

	err = execFFmpeg.run(ctx, duration, gifArgs...)
	if err != nil {
		return fmt.Errorf(encodeGIFErrFmt, err)
	}

	return nil
}
