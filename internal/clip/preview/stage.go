// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"uuid"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

// Render encodes a preview and publishes it under its final name.
//
// Parameters:
//   - ctx: Cancellation and deadline for the passes.
//   - store: Destination for the finished preview.
//   - paths: Local layout the staged render is written into.
//   - ffmpeg: Runner used for detection and encoding.
//   - source: Source media path.
//   - output: Final path the preview is published under.
//   - req: Parsed request carrying the marks and encoding options.
//   - preserveHDR: Whether an HDR source is kept rather than tone mapped. It
//     comes from the request, because the clip is where the user decides.
//
// Returns:
//   - err: Non-nil when the preview could not be written or published.
func Render(
	ctx context.Context,
	store blob.Blob,
	paths blob.Paths,
	runner *ffmpeg.ExecFFmpeg,
	source, output string,
	req clip.Request,
	preserveHDR bool,
) error {
	rect := crop.CropRect{}
	start := timecode.FromSeconds(req.StartTime).Duration()
	length := timecode.FromSeconds(req.Duration).Duration()

	if req.CropBlackBars {
		detected, err := runner.DetectCrop(ctx, source, start, length)
		if err != nil {
			// The preview id includes the crop flag, so publishing an uncropped
			// file here would be served as the cropped one until the source changes.
			return fmt.Errorf("detect crop: %w", err)
		}

		rect = detected
	}

	staged := paths.PreviewPath(uuid.New().String())

	err := runner.ExtractPreview(
		ctx,
		source,
		staged,
		start,
		length,
		req.AudioIndex,
		rect,
		clip.QualityPreset{
			WebSafeColor: clip.Flag(req.WebSafeColor),
			PreserveHDR:  preserveHDR,
		},
	)
	if err != nil {
		DiscardStaged(staged)

		// media names the operation, so this only adds which output it was
		// writing, which is what tells two concurrent previews apart.
		return fmt.Errorf("preview %s: %w", filepath.Base(output), err)
	}

	err = os.Rename(staged, output)
	if err != nil {
		DiscardStaged(staged)

		return fmt.Errorf("publish preview: %w", err)
	}

	err = store.Put(ctx, output)
	if err != nil {
		// The rename published the file locally, so it is now a cache hit for
		// every later request even though it never reached the bucket. That
		// preview would vanish on restart or from another instance, so the
		// upload failure is undone rather than left behind to look valid.
		discardErr := DiscardPublished(store, output)
		if discardErr != nil {
			return fmt.Errorf("upload preview: %w%w", err, discardErr)
		}

		return fmt.Errorf("upload preview: %w", err)
	}

	return nil
}

// DiscardPublished removes a published preview that failed to upload.
//
// Parameters:
//   - store: Store the preview was published through.
//   - output: Published path to remove.
//
// Returns:
//   - err: The cleanup failure, or nil when nothing needed removing.
func DiscardPublished(store blob.Blob, output string) error {
	err := store.DeleteFile(output)
	if err != nil && !os.IsNotExist(err) {
		logging.Logger.Warn().
			Str("path", output).
			Err(err).
			Msg("failed to remove unpublished preview")

		return fmt.Errorf("; discarding the published copy also failed: %w", err)
	}

	return nil
}

// DiscardStaged removes a staged preview that never reached its final name, so
// it cannot be served as a preview or mistaken for one.
//
// Parameters:
//   - staged: Path of the staged file.
func DiscardStaged(staged string) {
	err := os.Remove(staged)
	if err != nil && !os.IsNotExist(err) {
		logging.Logger.Warn().Str("path", staged).Err(err).Msg("failed to remove staged preview")
	}
}
