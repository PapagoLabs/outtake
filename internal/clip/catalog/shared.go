// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// TitleSource reports whether a viewer's Plex account sees a title.
type TitleSource interface {
	// Sees reports whether the viewer sees the title.
	Sees(ctx context.Context, viewer identity.Viewer, mediaID string) bool
}

// titleCheckLimit is how many titles a shared list asks Plex about at once.
const titleCheckLimit = 4

// SeenBy returns the check of which source titles a viewer sees in Plex.
//
// Parameters:
//   - source: Asks Plex about a title.
//   - viewer: The account the caller reads Plex as.
//
// Returns:
//   - check: Reports whether the viewer sees a title.
func SeenBy(source TitleSource, viewer identity.Viewer) TitleCheck {
	return func(ctx context.Context, mediaID string) bool {
		return source.Sees(ctx, viewer, mediaID)
	}
}

// Share shares a clip with the installation's other users, or makes it
// private again.
//
// The change goes through the queue, which saves it, so every read and the
// next start see it.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue the clip is changed through.
//   - store: Persistence handle for the clips table.
//   - id: Clip to change.
//   - shared: Whether the clip is shared.
//
// Returns:
//   - job: The changed clip.
//   - err: ErrClipNotFound when neither the queue nor the store has the clip,
//     or the wrapped read failure.
func Share(
	ctx context.Context,
	work *queue.Queue,
	store *database.DB,
	id string,
	shared bool,
) (*clip.Job, error) {
	err := adopt(ctx, work, store, id)
	if err != nil {
		return nil, fmt.Errorf("adopt: %w", err)
	}

	job := work.SetShared(id, shared)
	if job == nil {
		return nil, fmt.Errorf("share: %w", ErrClipNotFound)
	}

	return job, nil
}

// Shared returns every shared clip a caller may see, preferring the queue.
//
// A caller sees their own shared clips, and another account's when they can
// see its source title in Plex. The owner and admins see every shared clip.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the jobs that are queued or rendering.
//   - store: Persistence handle for the clips table.
//   - scope: Whose clips the caller may change.
//   - sees: Reports whether the caller can see a source title.
//
// Returns:
//   - jobs: The shared clips the caller may see, in the order they are kept.
func Shared(
	ctx context.Context,
	work *queue.Queue,
	store *database.DB,
	scope clip.Scope,
	sees TitleCheck,
) []*clip.Job {
	everyone := clip.Scope{Owner: scope.Owner, Everyone: true}

	return keepSeen(ctx, Jobs(ctx, work, store, everyone), scope, sees)
}

// SharedForMedia returns the shared clips of other accounts cut from one
// source that a caller may see.
//
// The caller's own clips, and every clip for the owner and admins, are listed
// by ForMedia, so none of them is listed here as well.
//
// Parameters:
//   - ctx: Request context.
//   - store: Persistence handle for the clips table.
//   - scope: Whose clips the caller may change.
//   - sees: Reports whether the caller can see a source title.
//   - mediaID: Plex rating key of the source item.
//
// Returns:
//   - jobs: Other accounts' shared clips of the source, nil when the caller
//     cannot see it or the clips cannot be read.
func SharedForMedia(
	ctx context.Context,
	store *database.DB,
	scope clip.Scope,
	sees TitleCheck,
	mediaID string,
) []*clip.Job {
	jobs, err := store.ListClipsForMedia(ctx, mediaID)
	if err != nil {
		return nil
	}

	others := make([]*clip.Job, 0, len(jobs))

	for _, job := range jobs {
		if job.Shared && !scope.Includes(job.OwnerID) {
			others = append(others, job)
		}
	}

	return keepSeen(ctx, others, scope, sees)
}

// keepSeen keeps the shared clips a caller may see.
//
// Each source title is asked about once, a few at a time, and only for
// another account's clip.
//
// Parameters:
//   - ctx: Request context.
//   - jobs: The clips to filter.
//   - scope: Whose clips the caller may change.
//   - sees: Reports whether the caller can see a source title, nil for none.
//
// Returns:
//   - kept: The shared clips the caller may see, in their order.
func keepSeen(
	ctx context.Context,
	jobs []*clip.Job,
	scope clip.Scope,
	sees TitleCheck,
) []*clip.Job {
	seen := seenTitles(ctx, othersTitles(jobs, scope), sees)
	kept := make([]*clip.Job, 0, len(jobs))

	for _, job := range jobs {
		if job.Shared && (scope.Includes(job.OwnerID) || seen[job.MediaID]) {
			kept = append(kept, job)
		}
	}

	return kept
}

// othersTitles lists the source titles of the shared clips a caller may not
// change, which a Plex question decides.
//
// Parameters:
//   - jobs: The clips.
//   - scope: Whose clips the caller may change.
//
// Returns:
//   - titles: Each title once, in the order the clips name them.
func othersTitles(jobs []*clip.Job, scope clip.Scope) []string {
	titles := make([]string, 0)

	for _, job := range jobs {
		if job.Shared && !scope.Includes(job.OwnerID) && !slices.Contains(titles, job.MediaID) {
			titles = append(titles, job.MediaID)
		}
	}

	return titles
}

// seenTitles asks which source titles a caller can see, titleCheckLimit at a
// time.
//
// Parameters:
//   - ctx: Request context.
//   - mediaIDs: The titles, without repeats.
//   - sees: Reports whether the caller can see a title, nil for none.
//
// Returns:
//   - seen: Each title the caller can see, mapped to true.
func seenTitles(ctx context.Context, mediaIDs []string, sees TitleCheck) map[string]bool {
	seen := make(map[string]bool, len(mediaIDs))
	if sees == nil {
		return seen
	}

	var (
		mu    sync.Mutex
		group sync.WaitGroup
	)

	slots := make(chan struct{}, titleCheckLimit)

	for _, mediaID := range mediaIDs {
		group.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()

			answer := sees(ctx, mediaID)

			mu.Lock()

			seen[mediaID] = answer
			mu.Unlock()
		})
	}

	group.Wait()

	return seen
}
