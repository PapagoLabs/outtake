// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"slices"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/failure"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/progress"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// progressInterval is the shortest gap between two recorded progress changes
// of one render.
const progressInterval = time.Second

// staleRenderAge is how long a staging file has to go unwritten before the
// startup sweep treats it as left by an interrupted render. ffmpeg writes its
// output continuously, so a live render's file is never this old.
const staleRenderAge = time.Hour

// persistProgress returns the callback that records a render's progress. It
// records at most one change per progressInterval, since each one is a
// database write, and the settled status records the final value anyway.
//
// Parameters:
//   - ctx: The job's context, canceled when the job is canceled or deleted.
//   - job: The job being rendered.
//   - jobQueue: The queue that owns the job.
//
// Returns:
//   - report: Callback for progress.WithProgress. ffmpeg's standard error is
//     read by one goroutine, so it is never called concurrently.
func persistProgress(ctx context.Context, job *clip.Job, jobQueue *queue.Queue) func(int) {
	var last time.Time

	return func(percent int) {
		now := time.Now()
		if !last.IsZero() && now.Sub(last) < progressInterval {
			return
		}

		last = now

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

// startQueue sweeps leftover render files, creates the worker queue, and
// restores persisted jobs.
//
// Parameters:
//   - ctx: The lifetime context the queue and its workers run under.
//   - cfg: The loaded configuration supplying the worker count.
//   - db: Database handle used to persist status and progress.
//   - ffmpeg: The FFmpeg runner every job executes through.
//   - store: The storage backend rendered output is uploaded to.
//   - paths: Output layout, which names a clip's file for each type.
//
// Returns:
//   - jobQueue: The started queue.
func startQueue(
	ctx context.Context,
	cfg *config.Config,
	db *database.DB,
	runner *ffmpeg.ExecFFmpeg,
	store blob.Blob,
	paths blob.Paths,
) *queue.Queue {
	sweepRenders(paths)

	var jobQueue *queue.Queue

	jobQueue = queue.NewQueue(cfg.NumWorkers, func(ctx context.Context, job *clip.Job) error {
		progressCtx := progress.WithProgress(ctx, persistProgress(ctx, job, jobQueue))

		err := processJob(progressCtx, job, runner, db, store, jobQueue)
		if err != nil {
			//nolint:wrapcheck // The error already names the step that failed, and the clip shows it as it is.
			return err
		}

		removeOtherOutputs(store, paths, job)

		return nil
	})

	jobQueue.SetStatusFunc(persistStatus(ctx, db))
	jobQueue.SetFailureFunc(failure.Describe)
	jobQueue.Start(ctx)

	// Submitting never waits on a worker, so the whole backlog is back in the
	// queue before the server listens and every request sees every clip.
	restoreJobs(ctx, db, jobQueue)

	return jobQueue
}

// removeOtherOutputs deletes a rendered clip's files for every other type, so
// a clip whose type changed does not keep the files it rendered before,
// including a video clip's SDR version.
//
// Parameters:
//   - store: The storage backend outputs live in.
//   - paths: Output layout, which names the clip's file for each type.
//   - job: The clip that just rendered.
func removeOtherOutputs(store blob.Blob, paths blob.Paths, job *clip.Job) {
	if job.OutputPath == "" {
		return
	}

	for _, kind := range clip.Types() {
		stale := paths.OutputPath(job.ID, kind)
		if stale == "" || stale == job.OutputPath {
			continue
		}

		removeStaleOutput(store, job.ID, stale)
	}
}

// removeStaleOutput deletes a clip's file of another type, and the SDR version
// beside it when that file was a video clip.
//
// Parameters:
//   - store: The storage backend outputs live in.
//   - id: The clip's id, for the log.
//   - stale: The file of the other type.
func removeStaleOutput(store blob.Blob, id, stale string) {
	for _, path := range []string{stale, clip.SDRPathFor(stale)} {
		if path == "" {
			continue
		}

		err := store.DeleteFile(path)
		if err != nil {
			log.Warn().Err(err).Str("job_id", id).Str("path", path).
				Msg("failed to remove an output of another clip type")
		}
	}
}

// sweepRenders removes the staging files and GIF palettes an interrupted
// render left in the output directories. It runs before this process starts a
// worker, and it only removes files nothing has written for staleRenderAge, so
// it spares a render another process sharing the storage path is running.
//
// Parameters:
//   - paths: Output layout naming the directories renders write to.
func sweepRenders(paths blob.Paths) {
	removed := ffmpeg.SweepStaged(
		time.Now().Add(-staleRenderAge),
		paths.ClipsDir(),
		paths.GifsDir(),
		paths.ScreenshotsDir(),
		paths.PreviewsDir(),
	)
	if removed > 0 {
		log.Info().Int("removed", removed).Msg("removed files left by interrupted renders")
	}
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
