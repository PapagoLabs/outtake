// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// stageReporter is the queue, told when a render moves on to its next encode.
type stageReporter interface {
	// SetStage records the encode now running.
	SetStage(id string, stage clip.Stage) *clip.Job
}

const (
	// sdrCRF is the x264 CRF an SDR version is encoded at.
	sdrCRF = 23
	// sdrEncoderPreset is the x264 preset an SDR version is encoded with,
	// quick because it is for watching in the app rather than for export.
	sdrEncoderPreset = "veryfast"
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
//   - rect: The black-bar crop, detected once for every encode of the job.
//
// Returns:
//   - error: Non-nil when the extract fails or the clip type is unknown.
func extractJob(
	ctx context.Context,
	job *clip.Job,
	runner *ffmpeg.ExecFFmpeg,
	db *database.DB,
	rect crop.CropRect,
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
			rect,
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
			rect,
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
			rect,
		)
		if err != nil {
			return fmt.Errorf("extract screenshot: %w", err)
		}
	default:
		return fmt.Errorf("%w: %s", errUnknownClipType, job.Type)
	}

	return nil
}

// processJob routes a job to the appropriate FFmpeg operation, then renders a
// video clip's SDR version when it needs one.
//
// Parameters:
//   - ctx: The job context, canceled when the job is canceled or deleted.
//   - job: The job being rendered.
//   - ffmpeg: The FFmpeg runner.
//   - db: Clip profile store, may be nil.
//   - store: The storage backend the rendered output is uploaded to.
//   - stages: The queue, told when the render moves on to the SDR version.
//
// Returns:
//   - error: Non-nil when the extract or the upload fails, or the job was
//     stopped while its SDR version rendered.
func processJob(
	ctx context.Context,
	job *clip.Job,
	runner *ffmpeg.ExecFFmpeg,
	db *database.DB,
	store blob.Blob,
	stages stageReporter,
) error {
	rect := detectJobCrop(ctx, runner, job)

	err := extractJob(ctx, job, runner, db, rect)
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

	err = renderSDRVersion(ctx, job, runner, db, store, stages, rect)
	if err != nil {
		return fmt.Errorf("sdr version: %w", err)
	}

	return nil
}

// renderSDRVersion renders the SDR version a video clip that keeps HDR from an
// HDR source plays on a screen or in a browser that cannot show its file. Any
// other video clip has no SDR version, so one left by an earlier render is
// removed. A failed SDR version is logged and removed, and the clip still
// completes, unless the job was stopped, which the queue settles itself.
//
// Parameters:
//   - ctx: The job context.
//   - job: The video clip just rendered.
//   - ffmpeg: The FFmpeg runner.
//   - db: Clip profile store, may be nil.
//   - store: The storage backend the SDR version is uploaded to.
//   - stages: The queue, told when the render moves on to the SDR version.
//   - rect: The crop the clip was rendered with.
//
// Returns:
//   - err: Non-nil only when the job was stopped during the SDR version.
func renderSDRVersion(
	ctx context.Context,
	job *clip.Job,
	runner *ffmpeg.ExecFFmpeg,
	db *database.DB,
	store blob.Blob,
	stages stageReporter,
	rect crop.CropRect,
) error {
	output := job.SDRPath()
	if output == "" {
		return nil
	}

	if !job.PreserveHDR || !sourceIsHDR(ctx, runner, job.InputPath) {
		removeSDRVersion(store, job.ID, output)

		return nil
	}

	stages.SetStage(job.ID, clip.StageSDR)

	err := runner.ExtractClip(
		ctx,
		job.InputPath,
		output,
		job.StartTime,
		job.Duration,
		sdrPreset(clipEncodePreset(ctx, db, &job.Clip)),
		job.AudioIndex,
		rect,
	)
	if err == nil {
		err = store.Put(ctx, output)
	}

	if err == nil {
		return nil
	}

	if ctx.Err() != nil {
		return fmt.Errorf("render: %w", err)
	}

	log.Warn().Err(err).Str("job_id", job.ID).Msg("failed to render the SDR version of a clip")
	removeSDRVersion(store, job.ID, output)

	return nil
}

// sdrPreset is how an SDR version is encoded: H.264 tone mapped to SDR at the
// SDR playback width, with the clip's own audio bitrate.
//
// Parameters:
//   - clipPreset: The clip's own encode settings.
//
// Returns:
//   - preset: The SDR version's encode settings.
func sdrPreset(clipPreset clip.QualityPreset) clip.QualityPreset {
	return clip.QualityPreset{
		CRF:         sdrCRF,
		Preset:      sdrEncoderPreset,
		AudioKbps:   clipPreset.AudioKbps,
		MaxWidth:    sdrPlaybackWidth(),
		PreserveHDR: false,
	}
}

// sdrPlaybackWidth is the width SDR versions are rendered at.
//
// Returns:
//   - width: The SDR playback width, 1080p.
func sdrPlaybackWidth() int {
	return clip.OutputWidth1080p
}

// sourceIsHDR reports whether a source carries an HDR transfer.
//
// Parameters:
//   - ctx: The job context.
//   - ffmpeg: The FFmpeg runner, whose probe is cached.
//   - input: The source media path.
//
// Returns:
//   - hdr: True when the source is HDR, false when it is SDR or unreadable.
func sourceIsHDR(ctx context.Context, runner *ffmpeg.ExecFFmpeg, input string) bool {
	info, err := runner.Probe(ctx, input)
	if err != nil {
		return false
	}

	return clip.IsHDRTransfer(info.ColorTransfer)
}

// removeSDRVersion deletes a clip's SDR version, logging a failure.
//
// Parameters:
//   - store: The storage backend the SDR version lives in.
//   - id: The clip's id, for the log.
//   - output: The SDR version's path.
func removeSDRVersion(store blob.Blob, id, output string) {
	err := store.DeleteFile(output)
	if err != nil {
		log.Warn().Err(err).Str("job_id", id).Str("path", output).
			Msg("failed to remove the SDR version of a clip")
	}
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
