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

// gifFrames is what a GIF renders: its window, size, rate, crop, and color.
type gifFrames struct {
	// start is the seek offset into the source.
	start time.Duration
	// duration is the length of the GIF window.
	duration time.Duration
	// width is the output width in pixels.
	width int
	// fps is the output frame rate.
	fps int
	// rect is the optional black-bar crop.
	rect crop.CropRect
	// toneMap is the HDR tone map chain, empty when none applies.
	toneMap string
}

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
//   - frames: Window, size, rate, crop, and tone map of the GIF.
//
// Returns:
//   - filter: crop, tone map, fps, scale, and yuv420p conversion.
func gifVideoChain(frames gifFrames) string {
	chain := gifScaleFilter(frames.width, frames.fps) + ",format=yuv420p"
	if frames.toneMap != "" {
		chain = frames.toneMap + "," + chain
	}

	return prependCrop(frames.rect, chain)
}

// gifPaletteFilter builds the palettegen -vf chain, with optional crop first.
//
// Parameters:
//   - frames: Window, size, rate, crop, and tone map of the GIF.
//
// Returns:
//   - filter: Palette generation -vf string.
func gifPaletteFilter(frames gifFrames) string {
	return gifVideoChain(frames) + ",palettegen=stats_mode=diff"
}

// gifEncodeFilter builds the paletteuse -filter_complex chain.
//
// Parameters:
//   - frames: Window, size, rate, crop, and tone map of the GIF.
//
// Returns:
//   - filter: Labeled paletteuse filter_complex string.
func gifEncodeFilter(frames gifFrames) string {
	return "[0:v]" + gifVideoChain(frames) +
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
		abortOnFlag,
		abortOnEmptyOutput,
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
//   - ctx: Cancellation and deadline for the encode.
//   - input: Source media path.
//   - output: Destination GIF path.
//   - start: Seek offset into the source.
//   - duration: Length of the GIF window.
//   - width: Output width in pixels, or 0 for the default.
//   - fps: Output frames per second, or 0 for the default.
//   - rect: Optional black-bar crop.
//
// An HDR source is always tone mapped to SDR before the palette is built,
// because a GIF cannot carry HDR and its untouched pixels would look washed out.
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

	cleanOutput, err := outputPath(output)
	if err != nil {
		return fmt.Errorf(encodeGIFErrFmt, err)
	}

	if width <= 0 {
		width = defaultWidth
	}
	if fps <= 0 {
		fps = defaultFPS
	}

	frames := gifFrames{
		start:    start,
		duration: duration,
		width:    width,
		fps:      fps,
		rect:     rect,
		toneMap:  execFFmpeg.sdrToneMap(ctx, cleanInput, start, duration),
	}

	err = publish(ctx, cleanOutput, func(staging string) error {
		return execFFmpeg.renderGIF(ctx, cleanInput, staging, frames)
	})
	if err != nil {
		return fmt.Errorf(encodeGIFErrFmt, err)
	}

	return nil
}

// renderGIF runs the palettegen and paletteuse passes into one file.
//
// Parameters:
//   - ctx: Cancellation and deadline for the passes.
//   - input: Absolute source media path.
//   - output: Absolute path the GIF is written to.
//   - frames: Window, size, rate, crop, and tone map of the GIF.
//
// Returns:
//   - err: Non-nil when either pass failed.
func (execFFmpeg *ExecFFmpeg) renderGIF(
	ctx context.Context,
	input, output string,
	frames gifFrames,
) error {
	palettePath := output + paletteSuffix
	// palettegen can create the PNG and then fail. The remove has to be armed
	// before that run, or the file is left next to the output.
	defer removeStaged(palettePath)

	err := execFFmpeg.run(
		ctx,
		frames.duration,
		gifPaletteArgs(
			execFFmpeg.ffmpegPath,
			input,
			palettePath,
			frames.start,
			frames.duration,
			gifPaletteFilter(frames),
		)...,
	)
	if err != nil {
		return fmt.Errorf("palettegen: %w", err)
	}

	err = execFFmpeg.run(ctx, frames.duration, gifEncodeArgs(
		execFFmpeg.ffmpegPath,
		input,
		palettePath,
		output,
		frames.start,
		frames.duration,
		gifEncodeFilter(frames),
	)...)
	if err != nil {
		return fmt.Errorf("paletteuse: %w", err)
	}

	return nil
}
