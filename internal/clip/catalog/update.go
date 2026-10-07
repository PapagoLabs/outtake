// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// queueEdit is the queue method an update goes through.
type queueEdit func(
	id string,
	edit clip.Edit,
	output string,
	save func(*clip.Job) error,
) (*clip.Job, bool, error)

// ErrClipNotFound reports an edit of a clip neither the queue nor the
// database has.
var ErrClipNotFound = errors.New("clip not found")

// Update applies a validated edit to a clip and saves it. The queue and the
// database change together, so every read sees the edit at once. A clip whose
// type changed is queued again, so its file matches its type.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the clip.
//   - store: Persistence handle for the clips table.
//   - paths: Output layout, which names the file a changed type renders to.
//   - id: Clip to change.
//   - edit: The validated change, with its type resolved.
//
// Returns:
//   - job: The edited clip.
//   - err: ErrClipNotFound, queue.ErrJobActive when the clip is rendering and
//     the edit would change its file, or the wrapped save or queue failure.
func Update(
	ctx context.Context,
	work *queue.Queue,
	store *database.DB,
	paths blob.Paths,
	id string,
	edit clip.Edit,
) (*clip.Job, error) {
	job, err := applyUpdate(ctx, store, paths, work, work.Edit, id, edit)
	if err != nil {
		return nil, fmt.Errorf("update clip: %w", err)
	}

	return job, nil
}

// UpdateAndRegenerate applies a validated edit to a clip, saves it, and
// renders the clip again.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue holding the clip.
//   - store: Persistence handle for the clips table.
//   - paths: Output layout, which names the file the clip renders to.
//   - id: Clip to change.
//   - edit: The validated change, with its type resolved.
//
// Returns:
//   - job: The edited clip, queued to render.
//   - err: ErrClipNotFound, queue.ErrJobActive when the clip is rendering, or
//     the wrapped save or queue failure.
func UpdateAndRegenerate(
	ctx context.Context,
	work *queue.Queue,
	store *database.DB,
	paths blob.Paths,
	id string,
	edit clip.Edit,
) (*clip.Job, error) {
	job, err := applyUpdate(ctx, store, paths, work, work.EditAndRegenerate, id, edit)
	if err != nil {
		return nil, fmt.Errorf("update and regenerate clip: %w", err)
	}

	return job, nil
}

// applyUpdate applies an edit through the queue, saves it, and queues the clip
// again when the queue says it has to render anew.
//
// Parameters:
//   - ctx: Request context.
//   - store: Persistence handle for the clips table.
//   - paths: Output layout.
//   - work: Queue holding the clip.
//   - apply: The queue method that applies the edit.
//   - id: Clip to change.
//   - edit: The validated change.
//
// Returns:
//   - job: The edited clip.
//   - err: As documented on Update.
func applyUpdate(
	ctx context.Context,
	store *database.DB,
	paths blob.Paths,
	work *queue.Queue,
	apply queueEdit,
	id string,
	edit clip.Edit,
) (*clip.Job, error) {
	err := adopt(ctx, work, store, id)
	if err != nil {
		return nil, fmt.Errorf("adopt: %w", err)
	}

	save := func(job *clip.Job) error {
		saveErr := store.SaveClip(ctx, job)
		if saveErr != nil {
			return fmt.Errorf("save clip: %w", saveErr)
		}

		return nil
	}

	job, rerender, err := apply(id, edit, paths.OutputPath(id, edit.Type), save)
	if errors.Is(err, queue.ErrJobNotFound) {
		return nil, fmt.Errorf("edit: %w", ErrClipNotFound)
	}

	if err != nil {
		return nil, fmt.Errorf("edit: %w", err)
	}

	if !rerender {
		return job, nil
	}

	job.OutputPath = paths.OutputPath(job.ID, job.Type)

	err = work.Requeue(job)
	if err != nil {
		return nil, fmt.Errorf("requeue: %w", err)
	}

	return job, nil
}

// adopt hands the queue a stored clip it does not hold yet, so the edit goes
// through one path whichever of the two had the clip.
//
// Parameters:
//   - ctx: Request context.
//   - work: Queue the clip is edited through.
//   - store: Persistence handle for the clips table.
//   - id: Clip to change.
//
// Returns:
//   - err: ErrClipNotFound when neither has the clip, or the wrapped read
//     failure.
func adopt(ctx context.Context, work *queue.Queue, store *database.DB, id string) error {
	if work.GetJob(id) != nil {
		return nil
	}

	stored, err := store.GetClip(ctx, id)
	if errors.Is(err, database.ErrClipNotFound) {
		return ErrClipNotFound
	}

	if err != nil {
		return fmt.Errorf("get clip: %w", err)
	}

	work.Restore(stored)

	return nil
}
