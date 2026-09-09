// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package wire

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/clip/storage"
	"github.com/PapagoLabs/outtake/internal/clip/worker"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
)

// StartQueue creates the worker queue and restores persisted jobs.
//
// Parameters:
//   - cfg: Application configuration.
//   - db: Database handle.
//   - ffmpeg: FFmpeg executor used for encode/probe work.
//   - store: Blob storage backend for clip artifacts.
//
// Returns:
//   - queue: The worker queue and restores persisted jobs.
func StartQueue(
	cfg *config.Config,
	db *database.DB,
	ffmpeg media.FFmpeg,
	store storage.Blob,
) *queue.Queue {
	jobQueue := queue.NewQueue(cfg.NumWorkers, func(ctx context.Context, job *queue.Job) error {
		progressCtx := media.WithProgress(ctx, func(percent int) {
			job.Progress = percent
			job.UpdatedAt = time.Now()

			saveErr := db.SaveClip(context.WithoutCancel(ctx), job)
			if saveErr != nil {
				log.Warn().Err(saveErr).Str("job_id", job.ID).Msg("failed to persist clip progress")
			}
		})

		return worker.ProcessJob(progressCtx, job, ffmpeg, db, store)
	})
	jobQueue.SetStatusFunc(func(job *queue.Job) {
		saveErr := db.SaveClip(context.Background(), job)
		if saveErr != nil {
			log.Warn().Err(saveErr).Str("job_id", job.ID).Msg("failed to persist clip status")
		}
	})
	jobQueue.Start()
	restoreJobs(db, jobQueue)

	return jobQueue
}

// restoreJobs reloads persisted clips into the in-memory queue.
//
// Parameters:
//   - db: Database handle.
//   - jobQueue: In-process clip job queue.
func restoreJobs(db ClipPersister, jobQueue *queue.Queue) {
	jobs, err := db.ListClips(context.Background())
	if err != nil {
		log.Warn().Err(err).Msg("failed to restore clips")

		return
	}

	for _, job := range jobs {
		switch job.Status {
		case queue.JobStatusPending, queue.JobStatusProcessing:
			job.Status = queue.JobStatusPending
			job.Error = ""
			jobQueue.Submit(job)
		default:
			jobQueue.Restore(job)
		}
	}
}
