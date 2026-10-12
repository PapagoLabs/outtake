// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// TitleCheck reports whether a caller can see a source title in Plex.
type TitleCheck func(ctx context.Context, mediaID string) bool

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

// Job finds one render a caller may change, preferring the queue.
//
// Another account's clip reads as missing, shared or not, so a caller cannot
// tell it exists.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the jobs that are queued or rendering.
//   - store: Persistence handle for the clips table.
//   - scope: Whose clips the caller may change.
//   - id: Job id to look up.
//
// Returns:
//   - The queued job or the stored job, or nil when neither has it or the
//     caller may not change it.
func Job(
	ctx context.Context,
	work *queue.Queue,
	store *database.DB,
	scope clip.Scope,
	id string,
) *clip.Job {
	job := find(ctx, work, store, id)
	if job == nil || !scope.Includes(job.OwnerID) {
		return nil
	}

	return job
}

// Viewable finds one render a caller may see, preferring the queue.
//
// A caller sees every clip they may change, and another account's shared
// clip when they can see its source title in Plex. Any other clip reads as
// missing.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the jobs that are queued or rendering.
//   - store: Persistence handle for the clips table.
//   - scope: Whose clips the caller may change.
//   - sees: Reports whether the caller can see a source title.
//   - id: Job id to look up.
//
// Returns:
//   - The job, or nil when it is missing or the caller may not see it.
func Viewable(
	ctx context.Context,
	work *queue.Queue,
	store *database.DB,
	scope clip.Scope,
	sees TitleCheck,
	id string,
) *clip.Job {
	job := find(ctx, work, store, id)
	if job == nil || !visible(ctx, scope, sees, job) {
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

// find looks a render up, preferring the queue.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the jobs that are queued or rendering.
//   - store: Persistence handle for the clips table.
//   - id: Job id to look up.
//
// Returns:
//   - job: The queued job or the stored job, or nil when neither has it.
func find(ctx context.Context, work *queue.Queue, store *database.DB, id string) *clip.Job {
	if job := work.GetJob(id); job != nil {
		return job
	}

	stored, err := store.GetClip(ctx, id)
	if err != nil {
		return nil
	}

	return stored
}

// visible reports whether a caller may see a render.
//
// The checks run cheapest first, so a private clip never costs a Plex
// request.
//
// Parameters:
//   - ctx: Request context.
//   - scope: Whose clips the caller may change.
//   - sees: Reports whether the caller can see a source title, nil for none.
//   - job: The render.
//
// Returns:
//   - seen: True for a clip the caller may change, or another account's
//     shared clip whose title the caller can see.
func visible(ctx context.Context, scope clip.Scope, sees TitleCheck, job *clip.Job) bool {
	if scope.Includes(job.OwnerID) {
		return true
	}

	return job.Shared && sees != nil && sees(ctx, job.MediaID)
}
