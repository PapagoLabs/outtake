// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"context"
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
)

// cropdetectPass names the crop detection pass in logs.
const cropdetectPass = "cropdetect"

// DetectCrop samples the source with cropdetect and returns a crop rectangle.
//
// Parameters:
//   - ctx: Cancellation and deadline for the pass.
//   - input: Source media path.
//   - start: Seek offset into the source.
//   - duration: Length of the clip.
//
// Returns:
//   - rect: The detected crop rectangle, or a zero value when there are no bars.
//   - err: Non-nil only when the pass failed and produced no usable output.
func (execFFmpeg *ExecFFmpeg) DetectCrop(
	ctx context.Context,
	input string,
	start, duration time.Duration,
) (crop.CropRect, error) {
	cleanInput, err := mediaPath(input)
	if err != nil {
		return crop.CropRect{}, fmt.Errorf("%s: %w", cropdetectPass, err)
	}

	window := crop.SampleDuration(duration)
	key, _ := probe.KeyFor(cleanInput)

	if rect, found := key.CachedCrop(start, window); found {
		return rect, nil
	}

	log, runErr := execFFmpeg.runStderr(
		ctx,
		cropdetectPass,
		crop.DetectArgs(execFFmpeg.ffmpegPath, cleanInput, start, duration)...,
	)

	detected, parsed := crop.ParseCropdetect(log)

	switch {
	case runErr != nil && !parsed:
		return crop.CropRect{}, fmt.Errorf("%s: %w", cropdetectPass, runErr)
	case !parsed || !detected.TrimsLog(log):
		// No bars is a result rather than a failure, so it is cached. A failed
		// pass is not, so a transient failure is retried next time.
		if runErr == nil {
			probe.StoreCrop(cleanInput, key, start, window, crop.CropRect{})
		}

		return crop.CropRect{}, nil
	case runErr == nil:
		probe.StoreCrop(cleanInput, key, start, window, detected)
	default:
		// The pass errored but its output parsed into a real rectangle. That is
		// not a result to keep, so it is returned without being cached.
	}

	return detected, nil
}
