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
	// qualityFlag is the FFmpeg quality flag.
	qualityFlag = "-q:v"
	// encodeScreenshotErrFmt is the message screenshot encode failures are wrapped with.
	encodeScreenshotErrFmt = "encode screenshot: %w"
	// stillPeakWindow is how much of the source after a still its PQ peak is
	// sampled from, when the source carries no MaxCLL or mastering peak. A
	// still has no length of its own, and a short window keeps a brighter shot
	// later on from darkening it.
	stillPeakWindow = time.Second
)

// screenshotEncodeArgs builds the ffmpeg argv for a still frame.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination image path.
//   - timestamp: Offset into the source to grab the frame from.
//   - rect: Optional black-bar crop.
//   - toneMap: HDR tone map chain, empty when none applies.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func screenshotEncodeArgs(
	ffmpegPath, input, output string,
	timestamp time.Duration,
	rect crop.CropRect,
	toneMap string,
) []string {
	args := []string{
		ffmpegPath,
		outputFlag,
		abortOnFlag, abortOnEmptyOutput,
		ssFlag, timecode.FromDuration(timestamp).FormatSeconds(),
		inputFlag, input,
		framesFlag, "1",
		qualityFlag, "2",
		updateFlag, "1",
	}
	if filter := screenshotFilter(rect, toneMap); filter != "" {
		args = append(args, videoFilterFlag, filter)
	}

	return append(args, output)
}

// screenshotFilter builds the still's -vf chain: the crop, then the tone map.
//
// Parameters:
//   - rect: Optional black-bar crop.
//   - toneMap: HDR tone map chain, empty when none applies.
//
// Returns:
//   - filter: The chain, empty when there is nothing to apply.
func screenshotFilter(rect crop.CropRect, toneMap string) string {
	if toneMap == "" {
		if rect.Valid() {
			return rect.Filter()
		}

		return ""
	}

	return prependCrop(rect, toneMap)
}

// ExtractScreenshot extracts a screenshot from a video.
//
// Parameters:
//   - ctx: Cancellation and deadline for the encode.
//   - input: Source media path.
//   - output: Destination image path.
//   - timestamp: Offset into the source to grab the frame from.
//   - rect: Optional black-bar crop.
//
// An HDR source is always tone mapped to SDR, because a JPEG cannot carry HDR
// and its untouched pixels would look washed out.
//
// Returns:
//   - err: Non-nil when the still could not be written.
func (execFFmpeg *ExecFFmpeg) ExtractScreenshot(
	ctx context.Context,
	input, output string,
	timestamp time.Duration,
	rect crop.CropRect,
) error {
	// Build and run the screenshot ffmpeg command.
	cleanInput, err := mediaPath(input)
	if err != nil {
		return fmt.Errorf(encodeScreenshotErrFmt, err)
	}

	cleanOutput, err := outputPath(output)
	if err != nil {
		return fmt.Errorf(encodeScreenshotErrFmt, err)
	}

	toneMap := execFFmpeg.sdrToneMap(ctx, cleanInput, timestamp, stillPeakWindow)

	err = publish(ctx, cleanOutput, func(staging string) error {
		return execFFmpeg.run(
			ctx,
			0,
			screenshotEncodeArgs(
				execFFmpeg.ffmpegPath,
				cleanInput,
				staging,
				timestamp,
				rect,
				toneMap,
			)...,
		)
	})
	if err != nil {
		return fmt.Errorf(encodeScreenshotErrFmt, err)
	}

	return nil
}
