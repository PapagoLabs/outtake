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
)

// screenshotEncodeArgs builds the ffmpeg argv for a still frame.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination image path.
//   - timestamp: Offset into the source to grab the frame from.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func screenshotEncodeArgs(
	ffmpegPath, input, output string,
	timestamp time.Duration,
	rect crop.CropRect,
) []string {
	args := []string{
		ffmpegPath,
		outputFlag,
		abortOnFlag, abortOnEmptyOutput,
		ssFlag, timecode.FromDuration(timestamp).FormatSeconds(),
		inputFlag, input,
		framesFlag, "1",
		qualityFlag, "2",
	}
	if rect.Valid() {
		args = append(args, videoFilterFlag, rect.Filter())
	}

	return append(args, output)
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

	err = publish(ctx, cleanOutput, func(staging string) error {
		return execFFmpeg.run(
			ctx,
			0,
			screenshotEncodeArgs(execFFmpeg.ffmpegPath, cleanInput, staging, timestamp, rect)...,
		)
	})
	if err != nil {
		return fmt.Errorf(encodeScreenshotErrFmt, err)
	}

	return nil
}
