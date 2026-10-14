// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"errors"
	"fmt"
	"io"
	"slices"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/clip/quota"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	clipcard "github.com/PapagoLabs/outtake/internal/web/components/clip"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// bulkAction is an action the clip lists apply to several clips.
type bulkAction string

// bulkRequestError is a bulk request's refusal, worded for the page.
type bulkRequestError string

// bulkResult counts what a bulk action did.
type bulkResult struct {
	// limit is the render limit that stopped a regenerate, nil for none.
	limit error
	// done counts the clips the action changed.
	done int
	// skipped counts the clips the action did not apply to.
	skipped int
	// failed counts the clips the action could not change.
	failed int
	// notStarted counts the clips a reached limit left alone.
	notStarted int
}

// The bulk actions.
const (
	// bulkDelete deletes each clip and its files.
	bulkDelete bulkAction = "delete"
	// bulkRegenerate renders each clip again with its stored settings.
	bulkRegenerate bulkAction = "regenerate"
	// bulkCancel stops each clip that is waiting or rendering.
	bulkCancel bulkAction = "cancel"
	// bulkShare shares each clip with the installation's other users.
	bulkShare bulkAction = "share"
	// bulkUnshare makes each clip private again.
	bulkUnshare bulkAction = "unshare"
)

// Bulk request fields and bounds.
const (
	// fieldBulkAction names the action in a form post.
	fieldBulkAction = "action"
	// fieldBulkID names a clip in a form post, once per clip.
	fieldBulkID = "id"
	// fieldMediaID names the title whose failed clips Retry Failed renders.
	fieldMediaID = "mediaId"
	// maxBulkClips is the most clips one request may name.
	maxBulkClips = 200
)

// Refusals of a bulk request.
const (
	// errBulkAction refuses an action the lists do not offer.
	errBulkAction bulkRequestError = "That action can't be applied to clips"
	// errBulkNone refuses a request that names no clip.
	errBulkNone bulkRequestError = "Select at least one clip"
	// errBulkTooMany refuses a request that names more than maxBulkClips.
	errBulkTooMany bulkRequestError = "Select at most 200 clips at a time"
	// errBulkUnreadable refuses a body that cannot be read.
	errBulkUnreadable bulkRequestError = "Couldn't read the request"
)

// errBulkSkipped marks a clip a bulk action does not apply to.
var errBulkSkipped = errors.New("clip skipped")

// Bulk applies one action to the clips a list selected.
//
// Each clip goes through the checks the action makes for one clip, and a
// clip the caller may not change is skipped. A regenerate stops at the first
// render limit it reaches.
//
// Parameters:
//   - ctx: Form post or JSON body naming the action and the clips.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) Bulk(ctx fiber.Ctx) error {
	req, err := parseBulk(ctx)
	if err != nil {
		//nolint:wrapcheck // The helper writes the response itself.
		return respondWithUpdateFailure(ctx, newRejection(
			fiber.StatusBadRequest,
			api.InvalidRequest,
			view.NewNotice(err.Error()),
		))
	}

	action := bulkAction(req.Action)

	//nolint:wrapcheck // The helper writes the response itself.
	return writeBulk(ctx, action, handler.runBulk(ctx, action, req.Clips))
}

// RetryFailed renders the failed clips the caller may change again.
//
// A form field or query parameter naming a title keeps it to that title's
// clips. One request renders at most maxBulkClips of them, newest first, and
// another request renders the rest.
//
// Parameters:
//   - ctx: Request optionally naming a title.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) RetryFailed(ctx fiber.Ctx) error {
	failed := handler.failedJobs(ctx, mediaIDOf(ctx))

	failed = failed[:min(len(failed), maxBulkClips)]

	ids := make([]string, 0, len(failed))

	for _, job := range failed {
		ids = append(ids, job.ID)
	}

	//nolint:wrapcheck // The helper writes the response itself.
	return writeBulk(ctx, bulkRegenerate, handler.runBulk(ctx, bulkRegenerate, ids))
}

// Error returns the refusal's text.
//
// Returns:
//   - message: What the page shows.
func (message bulkRequestError) Error() string {
	return string(message)
}

// applyBulk applies an action to one clip the caller may change.
//
// Parameters:
//   - ctx: Request context.
//   - action: The action.
//   - job: The clip.
//
// Returns:
//   - err: errBulkSkipped when the action does not apply to the clip, a
//     reached limit, or the failure.
func (handler *Handler) applyBulk(ctx fiber.Ctx, action bulkAction, job *clipdom.Job) error {
	var err error

	switch action {
	case bulkDelete:
		err = handler.deleteJob(ctx, job)
	case bulkRegenerate:
		err = handler.regenerateStored(ctx, job)
	case bulkCancel:
		err = handler.cancelRender(job)
	case bulkShare, bulkUnshare:
		_, err = catalog.Share(
			ctx.Context(),
			handler.clipQueue,
			handler.db,
			job.ID,
			action == bulkShare,
		)
	default:
		err = errBulkSkipped
	}

	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}

	return nil
}

// cancelRender stops a clip that is waiting or rendering.
//
// Parameters:
//   - job: The clip.
//
// Returns:
//   - err: errBulkSkipped for a clip that is neither.
func (handler *Handler) cancelRender(job *clipdom.Job) error {
	if !handler.clipQueue.Cancel(job.ID) {
		return errBulkSkipped
	}

	return nil
}

// failedJobs returns the failed clips the caller may change.
//
// Parameters:
//   - ctx: Request whose caller the clips are kept to.
//   - mediaID: The title to keep to, empty for every title.
//
// Returns:
//   - jobs: The failed clips, newest first.
func (handler *Handler) failedJobs(ctx fiber.Ctx, mediaID string) []*clipdom.Job {
	var jobs []*clipdom.Job

	if mediaID != "" {
		jobs = catalog.ForMedia(ctx.Context(), handler.db, middleware.ClipScope(ctx), mediaID)
	} else {
		jobs = handler.listJobs(ctx)
	}

	return catalog.Apply(jobs, catalog.Normalize(catalog.ClipListQuery{
		Status: clipdom.StatusFailed,
	}))
}

// regenerateStored renders a clip again with the settings it has, within the
// caller's limits.
//
// Its source must still be one Plex shares with the caller, and Keep HDR is
// taken from its profile, as a regenerate from its card does. A clip the queue
// still holds as waiting starts no new render, so the limits leave it alone.
//
// Parameters:
//   - ctx: Request context.
//   - job: The clip.
//
// Returns:
//   - err: errBulkSkipped for a clip that is rendering or whose title the
//     caller no longer sees, a reached limit, or the failure.
func (handler *Handler) regenerateStored(ctx fiber.Ctx, job *clipdom.Job) error {
	err := handler.sources.CheckAccess(ctx.Context(), middleware.PlexViewer(ctx), job.MediaID)
	if errors.Is(err, library.ErrNotShared) {
		return errBulkSkipped
	}

	if err != nil {
		return fmt.Errorf("check access: %w", err)
	}

	edit := mergeEdit(job, clipdom.EditRequest{}, job.Type, job.Quality)

	edit.PreserveHDR = handler.profileKeepHDR(ctx.Context(), edit.Quality)

	err = handler.validateEdit(ctx.Context(), job.InputPath, edit, handler.clipCap(ctx))
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	submit := func() error {
		_, saveErr := catalog.UpdateAndRegenerate(
			ctx.Context(), handler.clipQueue, handler.db, handler.clipPaths, job.ID, edit,
		)
		if errors.Is(saveErr, queue.ErrJobActive) {
			return errBulkSkipped
		}

		//nolint:wrapcheck // The caller wraps what the submit returns.
		return saveErr
	}

	_, err = catalog.UpdateWaiting(
		ctx.Context(), handler.clipQueue, handler.db, handler.clipPaths, job.ID, edit,
	)
	if errors.Is(err, queue.ErrJobNotWaiting) {
		err = handler.limits.AdmitClip(ctx.Context(), middleware.RenderMember(ctx), submit)
	}

	if err != nil {
		return fmt.Errorf("regenerate: %w", err)
	}

	return nil
}

// runBulk applies an action to each clip in turn.
//
// A clip the caller may not change, or that is gone, counts as skipped. A
// reached render limit, or the request ending, stops the run, and the clips
// left count as not started.
//
// Parameters:
//   - ctx: Request whose caller the clips are kept to.
//   - action: The action.
//   - ids: The clips.
//
// Returns:
//   - result: What the action did.
func (handler *Handler) runBulk(ctx fiber.Ctx, action bulkAction, ids []string) bulkResult {
	result := bulkResult{limit: nil, done: 0, skipped: 0, failed: 0, notStarted: 0}
	scope := middleware.ClipScope(ctx)

	for index, id := range ids {
		if ctx.Context().Err() != nil {
			result.notStarted = len(ids) - index

			return result
		}

		job := handler.lookupJob(ctx.Context(), scope, id)
		if job == nil {
			result.skipped++

			continue
		}

		err := handler.applyBulk(ctx, action, job)
		if quota.IsLimit(err) {
			result.limit = err
			result.notStarted = len(ids) - index

			return result
		}

		result.count(err, job.ID)
	}

	return result
}

// writeBulk answers a bulk action.
//
// An htmx request gets the status line the list shows, and an API caller gets
// the counts.
//
// Parameters:
//   - ctx: Request context.
//   - action: The action.
//   - result: What it did.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func writeBulk(ctx fiber.Ctx, action bulkAction, result bulkResult) error {
	limitMessage := ""
	if result.limit != nil {
		limitMessage = respond.Fail(ctx, result.limit).Message
	}

	message := bulkMessage(action, result, limitMessage)

	if respond.IsHTMXRequest(ctx) {
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return clipcard.BulkStatus(message).Render(ctx.Context(), writer)
		})
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, api.BulkResponse{
		Action:     string(action),
		Message:    message,
		Done:       result.done,
		Skipped:    result.skipped,
		Failed:     result.failed,
		NotStarted: result.notStarted,
	})
}

// count records how one clip's action ended.
//
// A failure is logged, because the page shows only how many failed.
//
// Parameters:
//   - err: The action's error, nil when it changed the clip.
//   - id: The clip.
func (result *bulkResult) count(err error, id string) {
	switch {
	case err == nil:
		result.done++
	case errors.Is(err, errBulkSkipped):
		result.skipped++
	default:
		result.failed++

		logging.Logger.Warn().Str("job_id", id).Err(err).Msg("bulk clip action failed")
	}
}

// mediaIDOf reads the title a Retry Failed request keeps to.
//
// Parameters:
//   - ctx: Request carrying a form field or query parameter.
//
// Returns:
//   - mediaID: The title, empty for every title.
func mediaIDOf(ctx fiber.Ctx) string {
	if mediaID := ctx.FormValue(fieldMediaID); mediaID != "" {
		return mediaID
	}

	return ctx.Query(fieldMediaID)
}

// parseBulk reads a bulk request from a form post or a JSON body.
//
// Parameters:
//   - ctx: The request.
//
// Returns:
//   - req: The action and the clips, each named once.
//   - err: A bulkRequestError for an unreadable body, an unknown action, no
//     clip, or too many.
func parseBulk(ctx fiber.Ctx) (api.BulkRequest, error) {
	req := api.BulkRequest{Action: "", Clips: nil}

	if respond.IsFormRequest(ctx) {
		req.Action = ctx.FormValue(fieldBulkAction)

		for _, id := range ctx.Request().PostArgs().PeekMulti(fieldBulkID) {
			req.Clips = append(req.Clips, string(id))
		}
	} else if ctx.Bind().Body(&req) != nil {
		return api.BulkRequest{}, errBulkUnreadable
	}

	//nolint:wrapcheck // The refusal is the page's message as it is.
	return checkBulk(req)
}

// checkBulk refuses a bulk request the lists would never send.
//
// Parameters:
//   - req: The request as read.
//
// Returns:
//   - req: The request with each clip named once, in the order given.
//   - err: A bulkRequestError for an unknown action, no clip, or too many.
func checkBulk(req api.BulkRequest) (api.BulkRequest, error) {
	switch bulkAction(req.Action) {
	case bulkDelete, bulkRegenerate, bulkCancel, bulkShare, bulkUnshare:
	default:
		return api.BulkRequest{}, errBulkAction
	}

	req.Clips = distinctIDs(req.Clips)

	switch {
	case len(req.Clips) == 0:
		return api.BulkRequest{}, errBulkNone
	case len(req.Clips) > maxBulkClips:
		return api.BulkRequest{}, errBulkTooMany
	default:
		return req, nil
	}
}

// distinctIDs drops empty and repeated clip ids, keeping the first of each.
//
// The order is kept, so a regenerate that reaches a limit starts the clips
// in the order the list shows them.
//
// Parameters:
//   - ids: The ids as given.
//
// Returns:
//   - distinct: Each id once, in the order given.
func distinctIDs(ids []string) []string {
	distinct := make([]string, 0, len(ids))

	for _, id := range ids {
		if id != "" && !slices.Contains(distinct, id) {
			distinct = append(distinct, id)
		}
	}

	return distinct
}
