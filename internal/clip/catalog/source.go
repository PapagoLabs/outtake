// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// Jobs returns the renders the catalog knows about, preferring the queue.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the jobs that are queued or rendering.
//   - store: Persistence handle for the clips table.
//
// Returns:
//   - jobs: The queued jobs, or every stored job when nothing is queued. Nil
//     when neither is readable.
func Jobs(ctx context.Context, work *queue.Queue, store *database.DB) []*clip.Job {
	queued := work.GetAllJobs()
	if len(queued) > 0 {
		return queued
	}

	stored, err := store.ListClips(ctx)
	if err != nil {
		return nil
	}

	return stored
}

// Job finds one render, preferring the queue.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the jobs that are queued or rendering.
//   - store: Persistence handle for the clips table.
//   - id: Job id to look up.
//
// Returns:
//   - The queued job or the stored job, or nil when neither has it.
func Job(ctx context.Context, work *queue.Queue, store *database.DB, id string) *clip.Job {
	queued := work.GetJob(id)
	if queued != nil {
		return queued
	}

	stored, err := store.GetClip(ctx, id)
	if err != nil {
		return nil
	}

	return stored
}

// ForMedia returns the jobs cut from one source, in whatever order storage
// keeps them.
//
// Parameters:
//   - ctx: Request context.
//   - store: Persistence handle for the clips table.
//   - mediaID: Plex rating key of the source item.
//
// Returns:
//   - jobs: The jobs made from that source, or nil when they cannot be read.
func ForMedia(ctx context.Context, store *database.DB, mediaID string) []*clip.Job {
	jobs, err := store.ListClipsForMedia(ctx, mediaID)
	if err != nil {
		return nil
	}

	return jobs
}
