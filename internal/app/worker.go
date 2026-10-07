// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"slices"

	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/progress"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// persistProgress returns the callback that records a render's progress.
//
// Parameters:
//   - ctx: The job's context, canceled when the job is canceled or deleted.
//   - job: The job being rendered.
//   - jobQueue: The queue that owns the job.
//
// Returns:
//   - report: Callback for progress.WithProgress.
func persistProgress(ctx context.Context, job *clip.Job, jobQueue *queue.Queue) func(int) {
	return func(percent int) {
		saveProgress(ctx, job, jobQueue, percent)
	}
}

// saveProgress records how far a render has got, if the job still wants it.
// The queue reports the change through its status callback, which persists it.
//
// Parameters:
//   - ctx: The job's context, canceled when the job is canceled or deleted.
//   - job: The job being rendered.
//   - jobQueue: The queue that owns the job.
//   - percent: Progress so far.
func saveProgress(ctx context.Context, job *clip.Job, jobQueue *queue.Queue, percent int) {
	if ctx.Err() != nil {
		return
	}

	jobQueue.SetProgress(job.ID, percent)
}

// persistStatus returns the queue callback that writes a job's state.
//
// Parameters:
//   - ctx: The lifetime context. Writes outlive its cancellation, so a job
//     settled during shutdown is still recorded.
//   - db: Database handle.
//
// Returns:
//   - report: Callback for queue.SetStatusFunc.
func persistStatus(ctx context.Context, db *database.DB) queue.StatusFunc {
	return func(job *clip.Job) {
		saveErr := db.SaveClip(context.WithoutCancel(ctx), job)
		if saveErr != nil {
			log.Warn().Err(saveErr).Str("job_id", job.ID).Msg("failed to persist clip status")
		}
	}
}

// startQueue creates the worker queue and restores persisted jobs.
//
// Parameters:
//   - ctx: The lifetime context the queue and its workers run under.
//   - cfg: The loaded configuration supplying the worker count.
//   - db: Database handle used to persist status and progress.
//   - ffmpeg: The FFmpeg runner every job executes through.
//   - store: The storage backend rendered output is uploaded to.
//
// Returns:
//   - jobQueue: The started queue.
func startQueue(
	ctx context.Context,
	cfg *config.Config,
	db *database.DB,
	runner *ffmpeg.ExecFFmpeg,
	store blob.Blob,
) *queue.Queue {
	var jobQueue *queue.Queue

	jobQueue = queue.NewQueue(cfg.NumWorkers, func(ctx context.Context, job *clip.Job) error {
		progressCtx := progress.WithProgress(ctx, persistProgress(ctx, job, jobQueue))

		return processJob(progressCtx, job, runner, db, store)
	})

	jobQueue.SetStatusFunc(persistStatus(ctx, db))
	jobQueue.Start(ctx)

	// Submitting never waits on a worker, so the whole backlog is back in the
	// queue before the server listens and every request sees every clip.
	restoreJobs(ctx, db, jobQueue)

	return jobQueue
}

// restoreBinding loads the selected Plex server from config or the database.
//
// Parameters:
//   - cfg: The loaded configuration supplying the configured server URL.
//   - db: Database handle holding the last selected server.
//   - bind: The binding to set the server on.
func restoreBinding(cfg *config.Config, db *database.DB, bind *identity.Binding) {
	if server, ok := plex.ServerFromURL(cfg.PlexServerURL, cfg.PlexToken); ok {
		bind.Set(server)

		return
	}

	server, ok, err := db.SelectedServer(context.Background())
	if err != nil {
		log.Warn().Err(err).Msg("failed to load selected server")

		return
	}

	if ok {
		bind.Set(server)
	}
}

// restoreJobs reloads persisted clips into the in-memory queue. Unfinished
// clips are submitted again oldest first, so they render in the order they
// were made.
//
// Parameters:
//   - ctx: The lifetime context the queue runs under.
//   - db: Database handle holding the persisted clips.
//   - jobQueue: The queue to resubmit or restore into.
func restoreJobs(ctx context.Context, db *database.DB, jobQueue *queue.Queue) {
	jobs, err := db.ListClips(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to restore clips")

		return
	}

	unfinished := make([]*clip.Job, 0, len(jobs))

	for _, job := range jobs {
		switch job.Status {
		case clip.StatusPending, clip.StatusProcessing:
			unfinished = append(unfinished, job)
		default:
			jobQueue.Restore(job)
		}
	}

	// ListClips is newest first.
	slices.Reverse(unfinished)

	for _, job := range unfinished {
		job.Status = clip.StatusPending
		job.Error = ""

		err := jobQueue.Submit(job)
		if err != nil {
			log.Error().Err(err).Str("job_id", job.ID).Msg("failed to restore job")
		}
	}
}
