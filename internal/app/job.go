// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/playback"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// renderReporter is the queue, told when a render moves on to its next
// encode and what each file it published holds.
type renderReporter interface {
	// SetStage records the encode now running.
	SetStage(id string, stage clip.Stage) *clip.Job
	// SetOutputFormat records what the clip's own file holds.
	SetOutputFormat(id string, format clip.Format) *clip.Job
	// SetSDRFormat records what the clip's SDR version holds.
	SetSDRFormat(id string, format clip.Format) *clip.Job
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
//   - reporter: The queue, told when the render moves on to the SDR version
//     and what each published file holds.
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
	reporter renderReporter,
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

	if job.Type != clip.TypeClip {
		// A GIF or screenshot has no player, so a clip that was a video before
		// a type change keeps no format from its old file.
		reporter.SetOutputFormat(job.ID, clip.Format{})
		reporter.SetSDRFormat(job.ID, clip.Format{})

		return nil
	}

	// Recorded as soon as the file is published, so a job stopped during its
	// SDR version still describes the file it left.
	reporter.SetOutputFormat(job.ID, fileFormat(ctx, runner, job.OutputPath))

	err = renderSDRVersion(ctx, job, runner, db, store, reporter, rect)
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
//   - reporter: The queue, told when the render moves on to the SDR version
//     and what the SDR version holds.
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
	reporter renderReporter,
	rect crop.CropRect,
) error {
	output := job.SDRPath()
	if output == "" {
		return nil
	}

	needed, err := needsSDRVersion(ctx, job, runner)
	if err != nil {
		return fmt.Errorf("check source: %w", err)
	}

	if !needed {
		removeSDRVersion(store, job.ID, output)
		reporter.SetSDRFormat(job.ID, clip.Format{})

		return nil
	}

	reporter.SetStage(job.ID, clip.StageSDR)

	err = runner.ExtractClip(
		ctx,
		job.InputPath,
		output,
		job.StartTime,
		job.Duration,
		sdrPreset(clipEncodePreset(ctx, db, &job.Clip), playback.MaxPreviewWidth(ctx, db)),
		job.AudioIndex,
		rect,
	)
	if err == nil {
		err = store.Put(ctx, output)
	}

	if err == nil {
		reporter.SetSDRFormat(job.ID, fileFormat(ctx, runner, output))

		return nil
	}

	if ctx.Err() != nil {
		return fmt.Errorf("render: %w", err)
	}

	log.Warn().Err(err).Str("job_id", job.ID).Msg("failed to render the SDR version of a clip")
	removeSDRVersion(store, job.ID, output)
	reporter.SetSDRFormat(job.ID, clip.Format{})

	return nil
}

// fileFormat reads what a published file holds.
//
// Parameters:
//   - ctx: The job context.
//   - ffmpeg: The FFmpeg runner, whose probe is cached.
//   - path: The published file.
//
// Returns:
//   - format: The file's size and range, zero when it cannot be read.
func fileFormat(ctx context.Context, runner *ffmpeg.ExecFFmpeg, path string) clip.Format {
	info, err := runner.Probe(ctx, path)
	if err != nil {
		return clip.Format{}
	}

	return clip.Format{
		Width:  info.Width,
		Height: info.Height,
		HDR:    clip.IsHDRTransfer(info.ColorTransfer),
	}
}

// needsSDRVersion reports whether a video clip needs an SDR version: it keeps
// HDR and its source is HDR. A probe cut short by a stopped job reads as an
// SDR source, so the stop is reported rather than taken for a clip that needs
// no SDR version, which would remove the one it has.
//
// Parameters:
//   - ctx: The job context.
//   - job: The video clip.
//   - ffmpeg: The FFmpeg runner, whose probe is cached.
//
// Returns:
//   - needed: True when the clip needs an SDR version.
//   - err: Non-nil when the job stopped before the source was probed.
func needsSDRVersion(ctx context.Context, job *clip.Job, runner *ffmpeg.ExecFFmpeg) (bool, error) {
	if !job.PreserveHDR {
		return false, nil
	}

	if sourceIsHDR(ctx, runner, job.InputPath) {
		return true, nil
	}

	if ctx.Err() != nil {
		return false, fmt.Errorf("probe: %w", ctx.Err())
	}

	return false, nil
}

// sdrPreset is how an SDR version is encoded: H.264 tone mapped to SDR, no
// wider than the maximum preview width, with the clip's own audio bitrate.
//
// Parameters:
//   - clipPreset: The clip's own encode settings.
//   - width: The maximum preview width the settings chose.
//
// Returns:
//   - preset: The SDR version's encode settings.
func sdrPreset(clipPreset clip.QualityPreset, width int) clip.QualityPreset {
	return clip.QualityPreset{
		CRF:         sdrCRF,
		Preset:      sdrEncoderPreset,
		AudioKbps:   clipPreset.AudioKbps,
		MaxWidth:    width,
		PreserveHDR: false,
	}
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
