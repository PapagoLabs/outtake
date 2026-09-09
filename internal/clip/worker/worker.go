// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/clip/storage"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
)

// errUnknownJobType is returned when a job has an unrecognized type.
var errUnknownJobType = errors.New("unknown job type")

// extractJob runs the FFmpeg extract for a clip, GIF, or screenshot job.
func extractJob(
	ctx context.Context,
	job *queue.Job,
	ffmpeg media.FFmpeg,
	db *database.DB,
) error {
	switch job.Type {
	case queue.JobTypeClip:
		err := ffmpeg.ExtractClip(
			ctx,
			job.InputPath,
			job.OutputPath,
			job.StartTime,
			job.Duration,
			clipEncodePreset(ctx, db, job),
			job.AudioIndex,
			detectJobCrop(ctx, ffmpeg, job),
		)
		if err != nil {
			return fmt.Errorf("extract clip: %w", err)
		}
	case queue.JobTypeGIF:
		err := ffmpeg.ExtractGIF(
			ctx,
			job.InputPath,
			job.OutputPath,
			job.StartTime,
			job.Duration,
			job.Width,
			job.FPS,
			detectJobCrop(ctx, ffmpeg, job),
		)
		if err != nil {
			return fmt.Errorf("extract gif: %w", err)
		}
	case queue.JobTypeScreenshot:
		err := ffmpeg.ExtractScreenshot(
			ctx,
			job.InputPath,
			job.OutputPath,
			job.StartTime,
			detectJobCrop(ctx, ffmpeg, job),
		)
		if err != nil {
			return fmt.Errorf("extract screenshot: %w", err)
		}
	default:
		return fmt.Errorf("%w: %s", errUnknownJobType, job.Type)
	}

	return nil
}

// ProcessJob routes a job to the appropriate FFmpeg operation.
func ProcessJob(
	ctx context.Context,
	job *queue.Job,
	ffmpeg media.FFmpeg,
	db *database.DB,
	store storage.Blob,
) error {
	err := extractJob(ctx, job, ffmpeg, db)
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
func detectJobCrop(ctx context.Context, ffmpeg media.FFmpeg, job *queue.Job) media.CropRect {
	if !job.CropBlackBars {
		return media.CropRect{}
	}

	crop, err := ffmpeg.DetectCrop(ctx, job.InputPath, job.StartTime, job.Duration)
	if err != nil {
		return media.CropRect{}
	}

	return crop
}

// clipEncodePreset resolves quality settings and the per-clip web-safe color flag.
//
// Parameters:
//   - ctx: Database context.
//   - db: Clip profile store; may be nil.
//   - job: Clip job whose Quality and WebSafeColor are applied.
//
// Returns:
//   - preset: Encode settings with WebSafeColor copied from the job.
func clipEncodePreset(ctx context.Context, db *database.DB, job *queue.Job) media.QualityPreset {
	preset := clipPreset(ctx, db, job.Quality)

	preset.WebSafeColor = job.WebSafeColor

	return preset
}

// clipPreset resolves a stored quality id onto ffmpeg settings.
func clipPreset(ctx context.Context, db *database.DB, quality string) media.QualityPreset {
	if quality == "" {
		return defaultClipPreset(ctx, db)
	}

	return media.ResolvePreset(quality, func(id string) (media.QualityPreset, bool) {
		return lookupClipPreset(ctx, db, id)
	})
}

// defaultClipPreset loads the stored default profile, or the built-in medium preset.
func defaultClipPreset(ctx context.Context, db *database.DB) media.QualityPreset {
	if db == nil {
		return media.QualityPresets[media.ClipQualityMedium]
	}

	profile, err := db.DefaultClipProfile(ctx)
	if err != nil {
		return media.QualityPresets[media.ClipQualityMedium]
	}

	return media.NormalizePreset(profile.QualityPreset())
}

// lookupClipPreset loads one stored profile by id.
func lookupClipPreset(ctx context.Context, db *database.DB, id string) (media.QualityPreset, bool) {
	if db == nil {
		return media.QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}, false
	}

	profile, err := db.GetClipProfile(ctx, id)
	if err != nil {
		return media.QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}, false
	}

	return profile.QualityPreset(), true
}
