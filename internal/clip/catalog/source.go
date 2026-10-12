// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// Jobs returns the renders a caller may see, preferring the queue.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the jobs that are queued or rendering.
//   - store: Persistence handle for the clips table.
//   - scope: Whose clips the caller may see.
//
// Returns:
//   - jobs: The queued jobs, or every stored job when nothing is queued, kept
//     to the caller's scope. Nil when neither is readable.
func Jobs(
	ctx context.Context,
	work *queue.Queue,
	store *database.DB,
	scope clip.Scope,
) []*clip.Job {
	queued := work.GetAllJobs()
	if len(queued) > 0 {
		return scope.Filter(queued)
	}

	stored, err := store.ListClips(ctx)
	if err != nil {
		return nil
	}

	return scope.Filter(stored)
}

// Job finds one render a caller may see, preferring the queue.
//
// Another account's clip reads as missing, so a caller cannot tell it exists.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the jobs that are queued or rendering.
//   - store: Persistence handle for the clips table.
//   - scope: Whose clips the caller may see.
//   - id: Job id to look up.
//
// Returns:
//   - The queued job or the stored job, or nil when neither has it or the
//     caller may not see it.
func Job(
	ctx context.Context,
	work *queue.Queue,
	store *database.DB,
	scope clip.Scope,
	id string,
) *clip.Job {
	job := work.GetJob(id)
	if job == nil {
		stored, err := store.GetClip(ctx, id)
		if err != nil {
			return nil
		}

		job = stored
	}

	if !scope.Includes(job.OwnerID) {
		return nil
	}

	return job
}

// ForMedia returns the jobs a caller may see that were cut from one source,
// in whatever order storage keeps them.
//
// Parameters:
//   - ctx: Request context.
//   - store: Persistence handle for the clips table.
//   - scope: Whose clips the caller may see.
//   - mediaID: Plex rating key of the source item.
//
// Returns:
//   - jobs: The caller's jobs made from that source, or nil when they cannot
//     be read.
func ForMedia(
	ctx context.Context,
	store *database.DB,
	scope clip.Scope,
	mediaID string,
) []*clip.Job {
	jobs, err := store.ListClipsForMedia(ctx, mediaID)
	if err != nil {
		return nil
	}

	return scope.Filter(jobs)
}
