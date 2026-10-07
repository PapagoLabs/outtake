// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// errUnknownClipType is returned when a job's clip has an unrecognized export type.
var errUnknownClipType = errors.New("unknown clip type")

// extractJob runs the FFmpeg extract for a clip, GIF, or screenshot job.
//
// Parameters:
//   - ctx: The job context, canceled when the job is canceled or deleted.
//   - job: The job being rendered.
//   - ffmpeg: The FFmpeg runner.
//   - db: Clip profile store, may be nil.
//
// Returns:
//   - error: Non-nil when the extract fails or the clip type is unknown.
func extractJob(
	ctx context.Context,
	job *clip.Job,
	runner *ffmpeg.ExecFFmpeg,
	db *database.DB,
) error {
	switch job.Type {
	case clip.TypeClip:
		err := runner.ExtractClip(
			ctx,
			job.InputPath,
			job.OutputPath,
			job.StartTime,
			job.Duration,
			clipEncodePreset(ctx, db, &job.Clip),
			job.AudioIndex,
			detectJobCrop(ctx, runner, job),
		)
		if err != nil {
			return fmt.Errorf("extract clip: %w", err)
		}
	case clip.TypeGIF:
		err := runner.ExtractGIF(
			ctx,
			job.InputPath,
			job.OutputPath,
			job.StartTime,
			job.Duration,
			job.Width,
			job.FPS,
			detectJobCrop(ctx, runner, job),
			clip.QualityPreset{WebSafeColor: job.WebSafeColor},
		)
		if err != nil {
			return fmt.Errorf("extract gif: %w", err)
		}
	case clip.TypeScreenshot:
		err := runner.ExtractScreenshot(
			ctx,
			job.InputPath,
			job.OutputPath,
			job.StartTime,
			detectJobCrop(ctx, runner, job),
			clip.QualityPreset{WebSafeColor: job.WebSafeColor},
		)
		if err != nil {
			return fmt.Errorf("extract screenshot: %w", err)
		}
	default:
		return fmt.Errorf("%w: %s", errUnknownClipType, job.Type)
	}

	return nil
}

// processJob routes a job to the appropriate FFmpeg operation.
//
// Parameters:
//   - ctx: The job context, canceled when the job is canceled or deleted.
//   - job: The job being rendered.
//   - ffmpeg: The FFmpeg runner.
//   - db: Clip profile store, may be nil.
//   - store: The storage backend the rendered output is uploaded to.
//
// Returns:
//   - error: Non-nil when the extract fails or the upload fails.
func processJob(
	ctx context.Context,
	job *clip.Job,
	runner *ffmpeg.ExecFFmpeg,
	db *database.DB,
	store blob.Blob,
) error {
	err := extractJob(ctx, job, runner, db)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	if job.OutputPath == "" {
		return nil
	}

	err = store.Put(ctx, job.OutputPath)
	if err != nil {
		return fmt.Errorf("store output: %w", err)
	}

	return nil
}

// detectJobCrop runs cropdetect when the job requested black-bar trimming.
//
// Parameters:
//   - ctx: The job context, canceled when the job is canceled or deleted.
//   - ffmpeg: The FFmpeg runner.
//   - job: The job whose CropBlackBars flag decides whether detection runs.
//
// Returns:
//   - rect: The detected crop, empty when trimming is off or detection fails.
func detectJobCrop(ctx context.Context, runner *ffmpeg.ExecFFmpeg, job *clip.Job) crop.CropRect {
	if !job.CropBlackBars {
		return crop.CropRect{}
	}

	rect, err := runner.DetectCrop(ctx, job.InputPath, job.StartTime, job.Duration)
	if err != nil {
		return crop.CropRect{}
	}

	return rect
}
