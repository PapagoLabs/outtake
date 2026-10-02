// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"uuid"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/binding"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/queue"
	"github.com/PapagoLabs/outtake/internal/storage"
)

// ClipHandler handles clip-related requests.
type ClipHandler struct {
	clipQueue   *queue.Queue
	clipStorage storage.Blob
	db          *database.DB
	cfg         *config.Config
	bind        *binding.Binding
	product     string
	clientID    string
	previews    *previewGate
	previewJobs *previewRegistry
	// mediaDurationFn reports a source's length. It is a field rather than a
	// direct probe so the range bound can be driven from a test, which D8 forbids
	// reaching for an external binary to do.
	mediaDurationFn func(ctx context.Context, inputPath string) (float64, bool)
}

// clipErrorCode is the API error code a pre-persistence failure is reported
// under. It is its own type so the code cannot be confused with the path beside
// it in a return, and so a caller cannot pass a path where a code is expected.
type clipErrorCode string

const (
	// DefaultQuality is the default clip quality.
	defaultQuality = "medium"
	// DefaultMediaType is the default media type.
	defaultMediaType = "movie"
	// ErrorNotFound is the error code for not found.
	errorNotFound = "not_found"
	// MessageNotFound is the message for not found.
	messageNotFound = "clip not found"
	// ParamID is the parameter name for ID.
	paramID = "id"

	// GIFMinWidth is the lowest GIF export width accepted from the form.
	gifMinWidth = 120
	// GIFMaxWidth is the highest GIF export width accepted from the form.
	gifMaxWidth = 1920
	// GIFMinFPS is the lowest GIF frame rate accepted from the form.
	gifMinFPS = 5
	// GIFMaxFPS is the highest GIF frame rate accepted from the form.
	gifMaxFPS = 30
	// FormChecked is the value of a checked HTML checkbox.
	formChecked = "1"
	// PreviewWait is how long a preview waits for a free slot before giving up.
	//
	// Long enough that a second click queues behind the first rather than
	// failing, short enough that a queued request cannot outlive a browser's
	// patience or a canceled client.
	previewWait = 30 * time.Second
	// ErrorPreviewBusy is the error code for a preview refused at admission.
	errorPreviewBusy = "preview_busy"
	// ErrorMediaPath is the error code for a source that could not be resolved.
	errorMediaPath = "media_path"
	// ErrorJobActive is the error code for a clip already queued or rendering.
	//
	// A conflict rather than a failure: the request was well formed, it just
	// cannot be carried out while the current attempt is still running.
	errorJobActive = "job_active"
	// ErrorPreviewNotRunning is the error code for canceling a finished preview.
	errorPreviewNotRunning = "preview_not_running"
	// MessagePreviewNotRunning explains a cancel that changed nothing.
	messagePreviewNotRunning = "preview is not running"
)

var (
	// ErrNoPlexServer is returned when no PMS is selected.
	errNoPlexServer = errors.New("no plex server selected")

	// ErrInvalidDuration is returned when a clip duration is out of range.
	errInvalidDuration = errors.New("invalid duration")

	// ErrRangeOutsideMedia is returned when a selection reaches past the end of
	// the source. Without it an 11 hour start on a 2 hour film is a legal clip
	// length, so it is accepted, persisted, and then fails to render.
	errRangeOutsideMedia = errors.New("selection is outside the media")

	// ErrUnknownQuality is returned when a clip profile id is not recognized.
	errUnknownQuality = errors.New("unknown clip profile")

	// ErrInvalidClipType is returned when clipType is set but not recognized.
	errInvalidClipType = errors.New("clip type must be one of: clip, video, screenshot, gif")

	// ErrInvalidGIFWidth is returned when a GIF width is outside the form bounds.
	errInvalidGIFWidth = fmt.Errorf("gif width must be between %d and %d", gifMinWidth, gifMaxWidth)
	// ErrInvalidGIFFPS is returned when a GIF fps is outside the form bounds.
	errInvalidGIFFPS = fmt.Errorf("gif fps must be between %d and %d", gifMinFPS, gifMaxFPS)

	// ErrSourceUnreadable is returned when a preview's source cannot be read.
	errSourceUnreadable = errors.New("source file is not readable")

	// ErrPreviewBusy is returned when no preview slot frees in time.
	errPreviewBusy = errors.New("too many previews are already rendering")
)

// NewClipHandler creates a new clip handler.
func NewClipHandler(
	jobQueue *queue.Queue,
	store storage.Blob,
	db *database.DB,
	cfg *config.Config,
	bind *binding.Binding,
	product, clientID string,
) *ClipHandler {
	// Bundle queue, store, database, config, and Plex binding.
	return &ClipHandler{
		clipQueue:   jobQueue,
		clipStorage: store,
		db:          db,
		cfg:         cfg,
		bind:        bind,
		product:     product,
		clientID:    clientID,
		previews:    newPreviewGate(cfg.MaxConcurrentPreviews),
		previewJobs: newPreviewRegistry(),
	}
}

// Cancel stops a pending or processing clip.
func (handler *ClipHandler) Cancel(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return writeError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	if !handler.clipQueue.Cancel(id) {
		return writeError(ctx, fiber.StatusConflict, "not_cancellable", "clip is not running")
	}

	updated := handler.clipQueue.GetJob(id)
	if updated != nil {
		err := handler.db.SaveClip(ctx.Context(), updated)
		if err != nil {
			return writeError(ctx, fiber.StatusInternalServerError, persistFailed, err.Error())
		}

		job = updated
	}

	if isHTMXRequest(ctx) {
		ctx.Set("HX-Refresh", "true")

		return sendStatusCode(ctx, fiber.StatusOK)
	}

	if isFormRequest(ctx) {
		return redirectTo(ctx, clipReturnPath(job.MediaID))
	}

	return writeJSON(ctx, fiber.StatusOK, clipResponse(job))
}

// CancelPreview stops an in-flight preview render.
//
// Parameters:
//   - ctx: Incoming request.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *ClipHandler) CancelPreview(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)

	// Existence is checked separately from running, so an id nothing was ever
	// registered under is not reported as a render that already finished.
	if _, known := handler.previewJobs.get(id); !known {
		return writeError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	if !handler.previewJobs.cancel(id) {
		return writeError(
			ctx,
			fiber.StatusConflict,
			errorPreviewNotRunning,
			messagePreviewNotRunning,
		)
	}

	return writeJSON(ctx, fiber.StatusOK, fiber.Map{"id": id})
}

// Create handles the create clip request.
func (handler *ClipHandler) Create(ctx fiber.Ctx) error {
	req, err := parseClipRequest(ctx)
	if err != nil {
		return writeError(ctx, fiber.StatusBadRequest, invalidRequest, err.Error())
	}

	jobType, ok := NormalizeClipType(req.ClipType)
	if !ok {
		return writeError(
			ctx,
			fiber.StatusBadRequest,
			"invalid_clip_type",
			errInvalidClipType.Error(),
		)
	}

	err = handler.validateClipParams(jobType, req)
	if err != nil {
		return writeError(ctx, fiber.StatusBadRequest, invalidRequest, err.Error())
	}

	inputPath, failCode, resolveErr := handler.resolveNewClip(ctx, &req, jobType)
	if resolveErr != nil {
		return writeError(ctx, fiber.StatusBadRequest, string(failCode), resolveErr.Error())
	}

	job := buildJob(
		&req,
		jobType,
		inputPath,
		preserveHDRFor(req.PreserveHDR, handler.cfg.PreserveHDR),
	)
	assignOutputPaths(job, handler.clipStorage)
	applyDefaults(job)

	err = handler.db.SaveClip(ctx.Context(), job)
	if err != nil {
		return writeError(ctx, fiber.StatusInternalServerError, persistFailed, err.Error())
	}

	err = handler.clipQueue.Submit(job)
	if err != nil {
		//nolint:wrapcheck // the error becomes a response body, not a returned chain.
		return writeJobSubmitError(ctx, err)
	}

	if isFormRequest(ctx) {
		return redirectTo(ctx, clipReturnPath(req.MediaID))
	}

	return writeJSON(ctx, fiber.StatusCreated, clipResponse(job))
}

// Delete handles the delete clip request.
func (handler *ClipHandler) Delete(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return writeError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	if job.OutputPath != "" {
		err := handler.clipStorage.DeleteFile(job.OutputPath)
		if err != nil {
			return writeError(ctx, fiber.StatusInternalServerError, "delete_failed", err.Error())
		}
	}

	// The queue forgets the job first, so the tombstone is in place before the
	// row goes. The other order leaves a window: a status change landing between
	// the two writes the row back, and the delete silently does nothing.
	handler.clipQueue.Delete(id)

	err := handler.db.DeleteClip(ctx.Context(), id)
	if err != nil {
		// The queue has already forgotten the job, so it has to take it back.
		// Left out, the row that survived the failed delete has nothing owning
		// it: a cancel would not find it, and the next start would resubmit a
		// render this delete already stopped.
		handler.clipQueue.Reinstate(job)

		// The row still describes the job as it was, so it is brought into line
		// with what the queue now holds. A save failure is logged rather than
		// returned: the caller can act on the delete failing and not on a second
		// write, and swapping them would report the wrong problem.
		saveErr := handler.db.SaveClip(ctx.Context(), job)
		if saveErr != nil {
			logging.Logger.Warn().
				Str("job_id", id).
				Err(saveErr).
				Msg("failed to persist reinstated clip")
		}

		return writeError(ctx, fiber.StatusInternalServerError, "delete_failed", err.Error())
	}

	// 200 rather than 204: htmx ships 204 in its noSwap list, so a 204 makes it
	// skip the swap and the card's hx-swap="delete" never removes the row.
	return sendStatusCode(ctx, fiber.StatusOK)
}

// Download handles the download clip request.
func (handler *ClipHandler) Download(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return writeError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	if job.Status != queue.JobStatusCompleted {
		return writeError(ctx, fiber.StatusConflict, "not_ready", "clip is not ready for download")
	}

	if !handler.clipStorage.FileExists(job.OutputPath) {
		return writeError(
			ctx,
			fiber.StatusNotFound,
			"file_missing",
			"output file not found on disk",
		)
	}

	ctx.Attachment(downloadName(job))

	err := ctx.SendFile(job.OutputPath)
	if err != nil {
		return fmt.Errorf("send file: %w", err)
	}

	return nil
}

// GetStatus handles the get clip status request.
func (handler *ClipHandler) GetStatus(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return writeError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	return writeJSON(ctx, fiber.StatusOK, clipResponse(job))
}

// List handles the list clips request.
func (handler *ClipHandler) List(ctx fiber.Ctx) error {
	jobs := handler.listJobs(ctx.Context())
	clips := make([]api.ClipResponse, 0, len(jobs))

	for _, job := range jobs {
		clips = append(clips, clipResponse(job))
	}

	return writeJSON(ctx, fiber.StatusOK, fiber.Map{"clips": clips})
}

// Preview renders a short low-quality segment without saving a clip.
//
// The render runs in the background and the redirect is returned immediately,
// so the page is reached while the preview is still encoding. The id in the
// redirect is what a client polls, and it is stable for the same parameters, so
// a second submission while the first is still running joins that render rather
// than starting another.
func (handler *ClipHandler) Preview(ctx fiber.Ctx) error {
	req, err := parseClipRequest(ctx)
	if err != nil {
		return writeError(ctx, fiber.StatusBadRequest, invalidRequest, err.Error())
	}

	inputPath, err := handler.resolveInput(ctx.Context(), req.MediaID)
	if err != nil {
		return writeError(ctx, fiber.StatusBadRequest, errorMediaPath, err.Error())
	}

	// Resolved before the id is derived, because the id has to reflect the
	// setting the render will actually use, not only what the form sent.
	req.PreserveHDR = new(preserveHDRFor(req.PreserveHDR, handler.cfg.PreserveHDR))

	previewID, err := previewRequestID(req, inputPath)
	if err != nil {
		return writeError(ctx, fiber.StatusBadRequest, errorMediaPath, err.Error())
	}

	final := handler.clipStorage.PreviewPath(previewID)

	// A preview already rendered for these exact parameters is returned without
	// touching ffmpeg, so repeating the same selection costs nothing. It is
	// recorded as finished so the page it redirects to can poll a status rather
	// than an unknown id.
	if handler.clipStorage.FileExists(final) {
		handler.previewJobs.remember(previewID)

		return redirectTo(ctx, previewRedirect(req, previewID))
	}

	// The render detaches from this request, so the values it needs are captured
	// by the closure rather than read from the context after the response has
	// been written. None of them change once the request is parsed.
	//
	// Admission and registration happen together, so a burst of clicks cannot
	// each observe the same headroom and collectively overshoot the limit.
	admitted := handler.previewJobs.render(
		ctx.Context(),
		previewID,
		handler.previews.capacity(),
		func(renderCtx context.Context) error {
			return handler.renderPreviewInBackground(renderCtx, inputPath, final, req)
		},
	)
	if admitted == refusedFull {
		return writeError(
			ctx,
			fiber.StatusTooManyRequests,
			errorPreviewBusy,
			errPreviewBusy.Error(),
		)
	}

	return redirectTo(ctx, previewRedirect(req, previewID))
}

// previewRedirect builds the media item location a preview request returns to.
//
// Parameters:
//   - req: Parsed request carrying the clip bounds.
//   - previewID: Id the preview is rendered under.
//
// Returns:
//   - location: A path-only redirect target.
func previewRedirect(req api.ClipRequest, previewID string) string {
	return previewRedirectURL(
		req.MediaID,
		previewID,
		req.StartTime,
		req.StartTime+req.Duration,
		req.WebSafeColor,
		exportFormFromRequest(req),
	)
}

// PreviewStatus reports how far a preview render has got.
//
// Parameters:
//   - ctx: Incoming request.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *ClipHandler) PreviewStatus(ctx fiber.Ctx) error {
	view, ok := handler.previewJobs.get(ctx.Params(paramID))
	if !ok {
		return writeError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	payload := fiber.Map{
		"id":       view.ID,
		"status":   view.Status,
		"progress": view.Progress,
	}

	// The file is only served once it is published, so the URL is withheld until
	// then rather than pointing at something that is not there.
	if view.Status == queue.JobStatusCompleted && handler.clipStorage.FileExists(
		handler.clipStorage.PreviewPath(view.ID),
	) {

		payload["url"] = "/previews/" + view.ID
	}

	if view.Error != "" {
		payload["error"] = view.Error
	}

	return writeJSON(ctx, fiber.StatusOK, payload)
}

// renderPreview encodes a preview and publishes it under its final name.
//
// Ffmpeg writes to a unique staged path rather than the final one, because the
// final path is derived from the request: two renders of the same parameters
// would otherwise write the same file at once, and a render killed part way
// would leave a partial file that a later cache hit serves as if it were whole.
// Staging in the same directory keeps the publish a rename within one
// filesystem, which is atomic.
//
// Parameters:
//   - ctx: Cancellation and deadline for the passes.
//   - store: Destination for the finished preview.
//   - ffmpeg: Runner used for detection and encoding.
//   - inputPath: Source media path.
//   - output: Final path the preview is published under.
//   - req: Parsed request carrying the marks and encoding options.
//   - preserveHDR: Whether an HDR source is kept rather than tone mapped. It
//     comes from the request, because the clip is where the user decides.
//
// Returns:
//   - err: Non-nil when the preview could not be written or published.
func renderPreview(
	ctx context.Context,
	store storage.Blob,
	ffmpeg media.FFmpeg,
	inputPath, output string,
	req api.ClipRequest,
	preserveHDR bool,
) error {
	crop := media.CropRect{}

	if req.CropBlackBars {
		detected, err := ffmpeg.DetectCrop(ctx, inputPath, req.StartTime, req.Duration)
		if err == nil {
			crop = detected
		}
	}

	staged := store.PreviewPath(uuid.New().String())

	err := ffmpeg.ExtractPreview(
		ctx,
		inputPath,
		staged,
		req.StartTime,
		req.Duration,
		req.AudioIndex,
		crop,
		media.QualityPreset{
			WebSafeColor: derefBool(req.WebSafeColor),
			PreserveHDR:  preserveHDR,
		},
	)
	if err != nil {
		discardStagedPreview(staged)

		// media names the operation, so this only adds which output it was
		// writing, which is what tells two concurrent previews apart.
		return fmt.Errorf("preview %s: %w", filepath.Base(output), err)
	}

	err = os.Rename(staged, output)
	if err != nil {
		discardStagedPreview(staged)

		return fmt.Errorf("publish preview: %w", err)
	}

	err = store.Put(ctx, output)
	if err != nil {
		// The rename published the file locally, so it is now a cache hit for
		// every later request even though it never reached the bucket. That
		// preview would vanish on restart or from another instance, so the
		// upload failure is undone rather than left behind to look valid.
		return fmt.Errorf("upload preview: %w%w", err, discardPublishedPreview(store, output))
	}

	return nil
}

// discardPublishedPreview removes a published preview that failed to upload.
//
// Parameters:
//   - store: Store the preview was published through.
//   - output: Published path to remove.
//
// Returns:
//   - err: The cleanup failure, or nil when nothing needed removing.
func discardPublishedPreview(store storage.Blob, output string) error {
	err := store.DeleteFile(output)
	if err != nil && !os.IsNotExist(err) {
		logging.Logger.Warn().
			Str("path", output).
			Err(err).
			Msg("failed to remove unpublished preview")

		return fmt.Errorf("; discarding the published copy also failed: %w", err)
	}

	return nil
}

// discardStagedPreview removes a staged preview that never reached its final
// name, so it cannot be served as a preview or mistaken for one.
//
// Parameters:
//   - staged: Path of the staged file.
func discardStagedPreview(staged string) {
	err := os.Remove(staged)
	if err != nil && !os.IsNotExist(err) {
		logging.Logger.Warn().Str("path", staged).Err(err).Msg("failed to remove staged preview")
	}
}

// previewRedirectURL builds the media item location that carries a rendered
// preview and the submitted export form back to the form.
//
// Start and end are written with millisecond precision so the marks the user
// chose are the marks the form re-renders. Formatting them more coarsely shifts
// each mark on every preview round trip, which is invisible on a seconds-only
// view and corrupts an edit that is trying to land on an exact frame.
//
// The export form state travels with them because the redirect discards the
// current page. Anything left behind is rebuilt from its default, which
// silently resets the profile, audio track, GIF size, export type, and crop
// toggle the user already chose.
//
// Parameters:
//   - mediaID: Plex rating key of the source item.
//   - previewID: Storage id of the rendered preview.
//   - start: Start mark in seconds.
//   - end: End mark in seconds.
//   - webSafeColor: Web-safe color checkbox state, nil when the form omitted it.
//   - state: Export form state to carry across the redirect.
//
// Returns:
//   - location: A path-only redirect target.
func previewRedirectURL(
	mediaID, previewID string,
	start, end float64,
	webSafeColor *bool,
	state exportFormState,
) string {
	values := url.Values{}
	values.Set("preview", previewID)
	values.Set(queryStart, media.FormatSeconds(start))
	values.Set(queryEnd, media.FormatSeconds(end))
	values.Set(queryWebSafeColor, webSafeQueryValue(webSafeColor))
	state.applyToQuery(values)

	return "/media/item/" + mediaID + "?" + values.Encode()
}

// Update saves clip metadata and optionally regenerates the file.
func (handler *ClipHandler) Update(ctx fiber.Ctx) error {
	job := handler.lookupJob(ctx.Context(), ctx.Params(paramID))
	if job == nil {
		return writeError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	req, err := parseClipRequest(ctx)
	if err != nil {
		return writeError(ctx, fiber.StatusBadRequest, invalidRequest, err.Error())
	}

	err = handler.applyRequestQuality(ctx.Context(), &req)
	if err != nil {
		return writeError(ctx, fiber.StatusBadRequest, "invalid_quality", err.Error())
	}

	jobType, err := clipJobType(req.ClipType, job.Type)
	if err != nil {
		return writeError(ctx, fiber.StatusBadRequest, "invalid_clip_type", err.Error())
	}

	err = handler.validateNewClip(ctx.Context(), job.InputPath, jobType, req)
	if err != nil {
		return writeError(ctx, fiber.StatusBadRequest, invalidRequest, err.Error())
	}

	applyClipEdits(job, req)

	err = handler.db.SaveClip(ctx.Context(), job)
	if err != nil {
		return writeError(ctx, fiber.StatusInternalServerError, persistFailed, err.Error())
	}

	err = handler.maybeRegenerate(ctx, job)
	if err != nil {
		//nolint:wrapcheck // the error becomes a response body, not a returned chain.
		return writeJobSubmitError(ctx, err)
	}

	return redirectTo(ctx, clipReturnPath(job.MediaID))
}

// writeJobSubmitError reports a job the queue would not take.
//
// Only a duplicate id is a conflict. Anything else is a failure like any other,
// and reporting it as a conflict would tell the caller to retry a request that
// cannot succeed.
//
// Parameters:
//   - ctx: Request context.
//   - err: The failure from the queue.
//
// Returns:
//   - err: The response write result.
func writeJobSubmitError(ctx fiber.Ctx, err error) error {
	if errors.Is(err, queue.ErrJobActive) {
		return writeError(ctx, fiber.StatusConflict, errorJobActive, err.Error())
	}

	return writeError(ctx, fiber.StatusInternalServerError, persistFailed, err.Error())
}

// applyClipEdits writes editable clip fields onto a stored job.
func applyClipEdits(job *queue.Job, req api.ClipRequest) {
	jobType, ok := NormalizeClipType(req.ClipType)
	if ok {
		job.Type = jobType
	}

	if req.Name != "" {
		job.Name = req.Name
	}

	if req.Quality != "" {
		job.Quality = req.Quality
	}

	job.StartTime = req.StartTime
	job.Duration = req.Duration
	job.Width = req.Width
	job.FPS = req.FPS
	job.AudioIndex = req.AudioIndex
	job.CropBlackBars = req.CropBlackBars

	if req.WebSafeColor != nil {
		job.WebSafeColor = *req.WebSafeColor
	}

	job.UpdatedAt = time.Now()
}

// clipReturnPath sends form posts back to the source media item when possible.
func clipReturnPath(mediaID string) string {
	if mediaID != "" {
		return "/media/item/" + mediaID
	}

	return pathClips
}

// applyRequestQuality resolves a non-empty quality field onto a profile id.
func (handler *ClipHandler) applyRequestQuality(ctx context.Context, req *api.ClipRequest) error {
	if req.Quality == "" {
		return nil
	}

	quality, err := handler.resolveQuality(ctx, req.Quality)
	if err != nil {
		return fmt.Errorf("apply quality: %w", err)
	}

	req.Quality = quality

	return nil
}

// listJobs returns in-memory jobs, falling back to persisted clips.
func (handler *ClipHandler) listJobs(ctx context.Context) []*queue.Job {
	jobs := handler.clipQueue.GetAllJobs()
	if len(jobs) > 0 {
		return jobs
	}

	stored, err := handler.db.ListClips(ctx)
	if err != nil {
		return nil
	}

	return stored
}

// lookupJob finds a job in the queue or the database.
func (handler *ClipHandler) lookupJob(ctx context.Context, id string) *queue.Job {
	job := handler.clipQueue.GetJob(id)
	if job != nil {
		return job
	}

	stored, err := handler.db.GetClip(ctx, id)
	if err != nil {
		return nil
	}

	return stored
}

// maybeRegenerate re-queues a clip when the regenerate form flag is set.
func (handler *ClipHandler) maybeRegenerate(ctx fiber.Ctx, job *queue.Job) error {
	if ctx.FormValue("regenerate") != "1" {
		return nil
	}

	err := handler.queueRegenerate(job)
	if err != nil {
		return fmt.Errorf("regenerate: %w", err)
	}

	return nil
}

// mediaDuration probes the source for its length.
//
// The probe is cached by file identity, so a source already opened for the page
// or a preview costs a map lookup rather than an ffprobe.
//
// Parameters:
//   - ctx: Request context.
//   - inputPath: Resolved source media path.
//
// Returns:
//   - duration: The source length in seconds.
//   - ok: False when the probe could not run or reported no length.
func (handler *ClipHandler) mediaDuration(ctx context.Context, inputPath string) (float64, bool) {
	if handler.mediaDurationFn != nil {
		return handler.mediaDurationFn(ctx, inputPath)
	}

	ffmpeg := media.NewExecFFmpeg(handler.cfg.FFmpegPath, handler.cfg.FFprobePath)

	info, err := ffmpeg.Probe(ctx, inputPath)
	if err != nil || info.Duration <= 0 {
		return 0, false
	}

	return info.Duration, true
}

// queueRegenerate re-queues a clip after metadata changes.
//
// The reset belongs to the queue, which does it under the same lock as its own
// check. Doing it here first would write Pending, which is the state the check
// reads, and the clip would be refused for looking like a second job for its own
// id — while the row it had just saved said pending with nothing running it.
//
// Parameters:
//   - job: The clip to run again.
//
// Returns:
//   - err: ErrJobActive when the clip is already rendering or queued.
func (handler *ClipHandler) queueRegenerate(job *queue.Job) error {
	assignOutputPaths(job, handler.clipStorage)

	err := handler.clipQueue.Requeue(job)
	if err != nil {
		return fmt.Errorf("requeue: %w", err)
	}

	return nil
}

// renderPreviewInBackground renders a preview on a queued goroutine.
//
// Taking the render slot here rather than in the request means a request is
// never held while another render finishes, and a preview the user is not
// waiting on can queue. A slot that cannot be taken in time fails the render,
// which the client reads from the status endpoint rather than from the response
// that already returned.
//
// Parameters:
//   - ctx: Cancellation for the render, held by the registry.
//   - inputPath: Resolved source media path.
//   - final: Final path the preview is published under.
//   - req: Parsed request carrying the marks and encoding options.
//
// Returns:
//   - err: Non-nil when the preview could not be rendered.
func (handler *ClipHandler) renderPreviewInBackground(
	ctx context.Context,
	inputPath, final string,
	req api.ClipRequest,
) error {
	release, err := handler.previews.acquire(ctx, previewWait)
	if err != nil {
		return fmt.Errorf("preview slot: %w", err)
	}

	defer release()

	// Re-checked under the slot. A render that queued behind another of the
	// same parameters would otherwise encode a preview that now exists.
	if handler.clipStorage.FileExists(final) {
		return nil
	}

	err = renderPreview(
		ctx,
		handler.clipStorage,
		media.NewExecFFmpeg(handler.cfg.FFmpegPath, handler.cfg.FFprobePath),
		inputPath,
		final,
		req,
		preserveHDRFor(req.PreserveHDR, handler.cfg.PreserveHDR),
	)
	if err != nil {
		return fmt.Errorf("render preview: %w", err)
	}

	return nil
}

// resolveInput maps a media id onto a local filesystem path.
func (handler *ClipHandler) resolveInput(ctx context.Context, mediaID string) (string, error) {
	path, err := resolveMediaPath(
		ctx,
		handler.cfg,
		handler.bind,
		handler.product,
		handler.clientID,
		mediaID,
	)
	if err != nil {
		return "", fmt.Errorf("resolve input: %w", err)
	}

	return path, nil
}

// resolveMediaPath maps a media id onto a local filesystem path.
func resolveMediaPath(
	ctx context.Context,
	cfg *config.Config,
	bind *binding.Binding,
	product, clientID, mediaID string,
) (string, error) {
	// E2E tests pass a local file path as the media id.
	if cfg.Env == "e2e" {
		info, err := os.Stat(mediaID)
		if err == nil && !info.IsDir() {
			return mediaID, nil
		}
	}

	server, ok := bind.Get()
	if !ok {
		return "", errNoPlexServer
	}

	plexClient := plex.NewClient(plex.ClientConfig{
		Product:  product,
		ClientID: clientID,
		Token:    server.Token,
		Timeout:  0,
		BaseURL:  "",
	})

	path, err := plexClient.GetMediaPath(ctx, server, mediaID)
	if err != nil {
		return "", fmt.Errorf("resolve media path: %w", err)
	}

	return cfg.RemapMediaPath(path), nil
}

// resolveNewClip resolves a new clip's quality and source, then bounds the
// selection by the source's own length.
//
// The range check runs last because it is the only one that needs the resolved
// path, and it runs before anything is persisted because a rejected selection
// must not leave a row behind that a render is then going to fail on. An 11 hour
// start on a 2 hour film is a legal clip length, so no other check catches it.
//
// The three steps are one seam so the caller reads as resolve-then-persist, and
// each keeps its own code because API clients distinguish them.
//
// Parameters:
//   - ctx: Request context.
//   - req: Parsed request, updated with the resolved quality.
//
// Returns:
//   - inputPath: Resolved source media path.
//   - code: API error code for whatever failed.
//   - err: Non-nil when the clip may not be persisted.
func (handler *ClipHandler) resolveNewClip(
	ctx fiber.Ctx,
	req *api.ClipRequest,
	jobType queue.JobType,
) (string, clipErrorCode, error) {
	quality, err := handler.resolveQuality(ctx.Context(), req.Quality)
	if err != nil {
		return "", "invalid_quality", fmt.Errorf("resolve profile: %w", err)
	}

	req.Quality = quality

	inputPath, err := handler.resolveInput(ctx.Context(), req.MediaID)
	if err != nil {
		return "", errorMediaPath, fmt.Errorf("resolve input: %w", err)
	}

	err = handler.validateSelection(ctx.Context(), inputPath, jobType, *req)
	if err != nil {
		return "", invalidRequest, fmt.Errorf("validate range: %w", err)
	}

	return inputPath, "", nil
}

// resolveQuality maps an empty or named quality onto a stored profile id.
func (handler *ClipHandler) resolveQuality(ctx context.Context, quality string) (string, error) {
	if quality == "" {
		profile, err := handler.db.DefaultClipProfile(ctx)
		if err != nil {
			return "", fmt.Errorf("resolve quality: %w", err)
		}

		return profile.ID, nil
	}

	_, err := handler.db.GetClipProfile(ctx, quality)
	if err == nil {
		return quality, nil
	}

	if !errors.Is(err, database.ErrClipProfileNotFound) {
		return "", fmt.Errorf("resolve quality: %w", err)
	}

	if _, ok := media.QualityPresets[media.ClipQuality(quality)]; ok {
		return quality, nil
	}

	return "", errUnknownQuality
}

// validateClipParams enforces duration and GIF encoder bounds.
func (handler *ClipHandler) validateClipParams(jobType queue.JobType, req api.ClipRequest) error {
	err := handler.validateDuration(jobType, req.Duration)
	if err != nil {
		return fmt.Errorf("validate duration: %w", err)
	}

	err = validateGIFParams(jobType, req.Width, req.FPS)
	if err != nil {
		return fmt.Errorf("validate gif: %w", err)
	}

	return nil
}

// validateDuration enforces clip duration limits.
func (handler *ClipHandler) validateDuration(jobType queue.JobType, duration float64) error {
	if jobType == queue.JobTypeScreenshot {
		if duration < 0 {
			return fmt.Errorf("%w: must be zero or greater", errInvalidDuration)
		}

		return nil
	}

	maxDur := handler.cfg.MaxClipDurSec
	if maxDur <= 0 {
		maxDur = defaultMaxClipDur
	}

	if duration <= 0 {
		// A form post has its duration measured between two marks, so this can
		// only be an end that is not after the start. A JSON caller sends a
		// duration directly, so the wording has to describe the range rather than
		// name an end it may never have sent.
		return fmt.Errorf("%w: the range must be longer than zero", errInvalidDuration)
	}

	if duration > float64(maxDur) {
		return fmt.Errorf("%w: must be between 0 and %d seconds", errInvalidDuration, maxDur)
	}

	return nil
}

// validateNewClip runs every check that must pass before a clip is persisted.
//
// The range bound needs the resolved source, so it cannot join validateClipParams
// on the request alone. Both run before anything is written, because a rejected
// selection must not leave a row behind that a render is then going to fail on.
//
// Parameters:
//   - ctx: Request context.
//   - inputPath: Resolved source media path.
//   - jobType: Normalized job type.
//   - req: Parsed request carrying the marks and encoding options.
//
// Returns:
//   - err: Non-nil when the clip may not be persisted.
func (handler *ClipHandler) validateNewClip(
	ctx context.Context,
	inputPath string,
	jobType queue.JobType,
	req api.ClipRequest,
) error {
	err := handler.validateClipParams(jobType, req)
	if err != nil {
		return fmt.Errorf("validate params: %w", err)
	}

	err = handler.validateSelection(ctx, inputPath, jobType, req)
	if err != nil {
		return fmt.Errorf("validate range: %w", err)
	}

	return nil
}

// validateSelection enforces that a selection lies inside the source.
//
// The duration checks in validateDuration cannot catch this. A start of eleven
// hours on a two hour film is a perfectly legal clip *length*, so it passes
// every other bound, and the job is persisted before ffmpeg reports that it
// seeked past the end. The range is only wrong relative to the source, so the
// source's own length is the only thing that can reject it.
//
// A source that cannot be probed is not rejected. The render would fail, but so
// would any guess made here, and a wrong rejection is worse than a late one.
//
// Parameters:
//   - ctx: Request context.
//   - inputPath: Resolved source media path.
//   - req: Parsed request carrying the marks.
//
// Returns:
//   - err: Non-nil when the selection reaches past the end of the source.
func (handler *ClipHandler) validateSelection(
	ctx context.Context,
	inputPath string,
	jobType queue.JobType,
	req api.ClipRequest,
) error {
	mediaDuration, ok := handler.mediaDuration(ctx, inputPath)
	if !ok {
		// The length is unknown, so the bounds that need it cannot be applied.
		// The one that does not is still checked, by handing checkRange a zero
		// length: it rejects a negative start and declines to judge the rest.
		mediaDuration = 0
	}

	err := checkRange(req.StartTime, selectionDuration(jobType, req), mediaDuration)
	if err != nil {
		return fmt.Errorf("check range: %w", err)
	}

	return nil
}

// selectionDuration is the range length a job type is actually bounded by.
//
// It is separate from validateSelection so the choice is testable without a
// probe. Extracting it also means dropping the screenshot case fails a test
// rather than quietly widening the bound.
//
// Parameters:
//   - jobType: Normalized job type.
//   - req: Parsed request carrying the marks.
//
// Returns:
//   - duration: The range length to bound by, in seconds.
func selectionDuration(jobType queue.JobType, req api.ClipRequest) float64 {
	// A screenshot is a single frame at the start mark, so the end mark is
	// derived from the form but never used. Bounding it would reject a frame
	// that sits inside the source purely because the end past it does not.
	if jobType == queue.JobTypeScreenshot {
		return 0
	}

	return req.Duration
}

// checkRange reports whether a selection fits inside a source of the given
// length.
//
// It is separate from the probe so the bound can be exercised without one.
//
// Parameters:
//   - start: Selection start in seconds.
//   - duration: Selection length in seconds, zero for a screenshot.
//   - mediaDuration: Source length in seconds.
//
// Returns:
//   - err: Non-nil when the selection reaches past the end of the source.
func checkRange(start, duration, mediaDuration float64) error {
	// A negative start is rejected before the length is consulted. It is wrong
	// whatever the source turns out to be, so an unknown length is no reason to
	// let it through.
	if start < 0 {
		return fmt.Errorf("%w: the start must not be negative", errRangeOutsideMedia)
	}

	// With no length there is nothing to compare against, and guessing one would
	// reject valid selections rather than the invalid ones.
	if mediaDuration <= 0 {
		return nil
	}

	if start >= mediaDuration {
		return fmt.Errorf(
			"%w: the start is %s but the media is only %s long",
			errRangeOutsideMedia,
			media.FromSeconds(start).Short(),
			media.FromSeconds(mediaDuration).Short(),
		)
	}

	if end := start + duration; end > mediaDuration {
		return fmt.Errorf(
			"%w: the end is %s but the media is only %s long",
			errRangeOutsideMedia,
			media.FromSeconds(end).Short(),
			media.FromSeconds(mediaDuration).Short(),
		)
	}

	return nil
}

// clipJobType prefers the requested clip type, then the stored job type.
func clipJobType(clipType string, fallback queue.JobType) (queue.JobType, error) {
	if clipType == "" {
		return fallback, nil
	}

	jobType, ok := NormalizeClipType(clipType)
	if !ok {
		return "", errInvalidClipType
	}

	return jobType, nil
}

// validateGIFParams enforces the GIF width and fps bounds from the export form.
func validateGIFParams(jobType queue.JobType, width, fps int) error {
	if jobType != queue.JobTypeGIF {
		return nil
	}

	if width != 0 && (width < gifMinWidth || width > gifMaxWidth) {
		return errInvalidGIFWidth
	}

	if fps != 0 && (fps < gifMinFPS || fps > gifMaxFPS) {
		return errInvalidGIFFPS
	}

	return nil
}

// parseClipRequest binds JSON or form fields into a clip request.
func parseClipRequest(ctx fiber.Ctx) (api.ClipRequest, error) {
	if strings.Contains(ctx.Get(fiber.HeaderContentType), "json") {
		var req api.ClipRequest

		err := ctx.Bind().Body(&req)
		if err != nil {
			return api.ClipRequest{}, fmt.Errorf("bind json: %w", err)
		}

		return req, nil
	}

	start, err := formDuration(ctx, "startTime", "start")
	if err != nil {
		//nolint:wrapcheck // The error message names the field and the value the user has to correct.
		return api.ClipRequest{}, err
	}

	end, err := formDuration(ctx, "endTime", "end")
	if err != nil {
		//nolint:wrapcheck // The error message names the field and the value the user has to correct.
		return api.ClipRequest{}, err
	}

	// The marks the user typed are authoritative, so the duration is derived from
	// them rather than read from the hidden field the browser computes. That field
	// is a second source of truth which can disagree with the form the user is
	// looking at, and when it does the clip is silently the wrong length.
	//
	// The range is measured by subtracting the two marks as durations.
	//
	// An end that is absent or not after the start leaves the duration at zero,
	// which validation rejects for a clip and accepts for a screenshot, where the
	// duration is unused.
	var duration float64

	if end > start {
		duration = (end - start).Seconds()
	}

	return api.ClipRequest{
		Name:          ctx.FormValue("name"),
		MediaID:       ctx.FormValue("mediaId"),
		MediaTitle:    ctx.FormValue("mediaTitle"),
		MediaType:     ctx.FormValue("mediaType"),
		StartTime:     start.Seconds(),
		Duration:      duration,
		Quality:       ctx.FormValue("quality"),
		ClipType:      ctx.FormValue("clipType"),
		Width:         formInt(ctx, "width"),
		FPS:           formInt(ctx, "fps"),
		AudioIndex:    formInt(ctx, "audioIndex"),
		CropBlackBars: ctx.FormValue("cropBlackBars") == formChecked,
		WebSafeColor:  new(ctx.FormValue("webSafeColor") == formChecked),
		PreserveHDR:   new(ctx.FormValue("preserveHdr") == formChecked),
	}, nil
}

// derefBool returns the pointed value, or false when ptr is nil.
//
// Parameters:
//   - value: Optional boolean from JSON or form binding.
//
// Returns:
//   - result: *value when set, otherwise false.
//
// preserveHDRFor resolves the requested keep-HDR setting.
//
// A request that omits the field takes the configured default, which is what the
// field documents. An explicit false is the caller declining, and is honored
// rather than overwritten by a server-wide setting.
//
// Parameters:
//   - requested: The request's value, nil when the field was absent.
//   - fallback: The configured default.
//
// Returns:
//   - preserve: Whether the source's HDR transfer is kept.
func preserveHDRFor(requested *bool, fallback bool) bool {
	if requested == nil {
		return fallback
	}

	return *requested
}

// derefBool reads an optional flag, treating an absent one as false.
func derefBool(value *bool) bool {
	if value == nil {
		return false
	}

	return *value
}

// webSafeQueryValue encodes an optional web-safe color flag as a query value.
//
// Parameters:
//   - value: Optional checkbox from the preview form.
//
// Returns:
//   - raw: formChecked when true, otherwise queryUnchecked.
func webSafeQueryValue(value *bool) string {
	if derefBool(value) {
		return formChecked
	}

	return queryUnchecked
}

// formInt parses a form field as int, or 0.
func formInt(ctx fiber.Ctx, name string) int {
	value, err := strconv.Atoi(ctx.FormValue(name))
	if err != nil {
		return 0
	}

	return value
}

// formDuration parses a timecode form field as a duration.
//
// An empty field is zero. Zero is where a clip starting at the beginning of a
// source belongs, so leaving a mark blank is a choice rather than a mistake.
// A field holding anything other than a timecode is reported as an error naming
// the field and the value, so the form can say which of the two marks to correct.
//
// The value is kept as a duration rather than seconds so a range spanning two
// marks is measured by subtracting whole nanoseconds. Timecodes are millisecond
// aligned, so subtracting seconds happens to land on the right side of the limit
// today, but whole nanoseconds are exact by construction rather than by that
// alignment, and the form would not stay aligned if the format ever gained
// precision.
//
// Parameters:
//   - ctx: Request context.
//   - name: Form field holding the timecode.
//   - label: How the field is named to the user.
//
// Returns:
//   - duration: The parsed duration, zero when the field is empty.
//   - err: Non-nil when the field holds something that is not a timecode.
func formDuration(ctx fiber.Ctx, name, label string) (time.Duration, error) {
	value := ctx.FormValue(name)

	// media.Parse trims before it parses, so a spaces-only field would reach it
	// looking absent. Whether a mark was left blank or filled with spaces is a
	// distinction only this check can make.
	spacesOnly := value != "" && strings.TrimSpace(value) == ""

	tc, err := media.Parse(value)
	if err != nil || spacesOnly {
		return 0, fmt.Errorf(
			"%w: the %s must be a timecode such as 00:01:23.456, not %q",
			media.ErrInvalidTimecode, label, value,
		)
	}

	return tc.Duration(), nil
}

// clipName prefers the user-supplied name, then the media title.
func clipName(req *api.ClipRequest) string {
	if req.Name != "" {
		return req.Name
	}

	return req.MediaTitle
}

// downloadName builds a Content-Disposition filename for a completed clip.
func downloadName(job *queue.Job) string {
	base := job.Name
	if base == "" {
		base = job.MediaTitle
	}
	if base == "" {
		base = job.ID
	}

	base = strings.ReplaceAll(base, "/", "-")
	base = strings.ReplaceAll(base, "\\", "-")

	switch job.Type {
	case queue.JobTypeGIF:
		return base + ".gif"
	case queue.JobTypeScreenshot:
		return base + ".jpg"
	default:
		return base + ".mp4"
	}
}

// isFormRequest reports whether the request is urlencoded form data.
func isFormRequest(ctx fiber.Ctx) bool {
	return strings.Contains(ctx.Get(fiber.HeaderContentType), "application/x-www-form-urlencoded")
}

// buildJob constructs a pending queue job from a clip request.
func buildJob(
	req *api.ClipRequest,
	jobType queue.JobType,
	inputPath string,
	preserveHDR bool,
) *queue.Job {
	return &queue.Job{
		ID:            uuid.New().String(),
		Type:          jobType,
		Name:          clipName(req),
		MediaID:       req.MediaID,
		MediaTitle:    req.MediaTitle,
		MediaType:     req.MediaType,
		InputPath:     inputPath,
		OutputPath:    "",
		StartTime:     req.StartTime,
		Duration:      req.Duration,
		Quality:       req.Quality,
		Width:         req.Width,
		FPS:           req.FPS,
		AudioIndex:    req.AudioIndex,
		CropBlackBars: req.CropBlackBars,
		WebSafeColor:  derefBool(req.WebSafeColor),
		PreserveHDR:   preserveHDR,
		Status:        queue.JobStatusPending,
		Progress:      0,
		Error:         "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// clipResponse maps a job onto the public clip payload.
func clipResponse(job *queue.Job) api.ClipResponse {
	return api.ClipResponse{
		ID:            job.ID,
		Name:          job.Name,
		MediaID:       job.MediaID,
		MediaTitle:    job.MediaTitle,
		MediaType:     job.MediaType,
		ClipType:      string(job.Type),
		Status:        string(job.Status),
		Progress:      job.Progress,
		InputPath:     "",
		OutputPath:    "",
		Error:         job.Error,
		CreatedAt:     job.CreatedAt,
		UpdatedAt:     job.UpdatedAt,
		AudioIndex:    job.AudioIndex,
		CropBlackBars: job.CropBlackBars,
		WebSafeColor:  job.WebSafeColor,
		PreserveHDR:   job.PreserveHDR,
	}
}
