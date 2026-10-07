// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"errors"
	"fmt"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/respond"
)

// Handler handles clip-related requests.
type Handler struct {
	clipQueue   *queue.Queue
	clipStorage blob.Blob
	clipPaths   blob.Paths
	db          *database.DB
	cfg         *config.Config
	sources     *library.MediaSource
}

// New creates a new clip handler.
//
// Parameters:
//   - jobQueue: Queue carrying render jobs.
//   - store: Blob store for inputs and outputs.
//   - paths: Path layout for stored blobs.
//   - db: Clip database.
//   - cfg: Loaded configuration.
//   - sources: Resolver for the media a clip is cut from.
//
// Returns:
//   - handler: A clip handler wired to the supplied collaborators.
func New(
	jobQueue *queue.Queue,
	store blob.Blob,
	paths blob.Paths,
	db *database.DB,
	cfg *config.Config,
	sources *library.MediaSource,
) *Handler {
	return &Handler{
		clipQueue:   jobQueue,
		clipStorage: store,
		clipPaths:   paths,
		db:          db,
		cfg:         cfg,
		sources:     sources,
	}
}

// listJobs returns every clip the catalog knows about.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - jobs: The queued jobs, or the stored clips when no job is queued.
func (handler *Handler) listJobs(ctx context.Context) []*clipdom.Job {
	return catalog.Jobs(ctx, handler.clipQueue, handler.db)
}

// lookupJob finds a clip in the queue or the database.
//
// Parameters:
//   - ctx: Request context.
//   - id: Clip id to look up.
//
// Returns:
//   - job: The queued job or stored clip, or nil when neither is found.
func (handler *Handler) lookupJob(ctx context.Context, id string) *clipdom.Job {
	return catalog.Job(ctx, handler.clipQueue, handler.db, id)
}

// resolveInput maps a media id onto a local filesystem path.
//
// Parameters:
//   - ctx: Request context.
//   - mediaID: Plex rating key of the source item.
//
// Returns:
//   - path: Remapped local path for the media file.
//   - err: Non-nil when no server is selected or the lookup fails.
func (handler *Handler) resolveInput(ctx context.Context, mediaID string) (string, error) {
	path, err := handler.sources.Resolve(ctx, mediaID)
	if err != nil {
		return "", fmt.Errorf("resolve input: %w", err)
	}

	return path, nil
}

// resolveNewClip resolves a new clip's quality and source, then bounds the edit
// it describes by the source's own length.
//
// Parameters:
//   - ctx: Request context.
//   - req: Parsed request, updated with the resolved quality.
//   - jobType: Normalized job type.
//
// Returns:
//   - inputPath: Resolved source media path.
//   - code: API error code for whatever failed.
//   - err: Non-nil when the clip may not be persisted.
func (handler *Handler) resolveNewClip(
	ctx fiber.Ctx,
	req *api.ClipRequest,
	jobType clipdom.Type,
) (string, api.ErrorCode, error) {
	quality, err := profile.ResolveProfile(ctx.Context(), handler.db, req.Quality)
	if err != nil {
		return "", api.InvalidQuality, fmt.Errorf("resolve profile: %w", err)
	}

	req.Quality = quality

	inputPath, err := handler.resolveInput(ctx.Context(), req.MediaID)
	if err != nil {
		return "", api.MediaPathUnresolved, fmt.Errorf("resolve input: %w", err)
	}

	err = handler.validateEdit(ctx.Context(), inputPath, RequestEdit(*req, jobType))
	if err != nil {
		//nolint:wrapcheck // The handler writes the error as the response body.
		return "", api.InvalidRequest, err
	}

	return inputPath, "", nil
}

// validateEdit checks that an edit may be stored against the source it names.
//
// Parameters:
//   - ctx: Request context.
//   - inputPath: Resolved source media path.
//   - edit: The change the request describes, with its type resolved.
//
// Returns:
//   - err: Non-nil when the edit may not be persisted.
func (handler *Handler) validateEdit(
	ctx context.Context,
	inputPath string,
	edit clipdom.Edit,
) error {
	err := handler.sources.CheckEdit(
		ctx,
		inputPath,
		edit,
		clipdom.DurationCap(handler.cfg.MaxClipDur),
	)
	if err != nil {
		return fmt.Errorf("validate range: %w", err)
	}

	return nil
}

// submitFailure describes a clip the queue would not take.
//
// Parameters:
//   - err: The failure from the queue.
//
// Returns:
//   - failure: The rejection to report.
func submitFailure(err error) *clipRejection {
	if errors.Is(err, queue.ErrJobActive) {
		return newRejection(fiber.StatusConflict, api.JobActive, err.Error())
	}

	return newRejection(fiber.StatusInternalServerError, api.PersistFailed, err.Error())
}

// writeJobSubmitError reports a job the queue would not take.
//
// Parameters:
//   - ctx: Request context.
//   - err: The failure from the queue.
//
// Returns:
//   - err: The response write result.
func writeJobSubmitError(ctx fiber.Ctx, err error) error {
	failure := submitFailure(err)

	return respond.WriteError(ctx, failure.status, failure.code, failure.message)
}
