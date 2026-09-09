// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"uuid"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/clip/storage"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/binding"
	sharedclip "github.com/PapagoLabs/outtake/internal/web/handlers/shared/clip"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
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
}

const (
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
)

var (
	// ErrNoPlexServer is returned when no PMS is selected.
	errNoPlexServer = errors.New("no plex server selected")

	// ErrInvalidDuration is returned when a clip duration is out of range.
	errInvalidDuration = errors.New("invalid duration")

	// ErrUnknownQuality is returned when a clip profile id is not recognized.
	errUnknownQuality = errors.New("unknown clip profile")

	// ErrInvalidClipType is returned when clipType is set but not recognized.
	errInvalidClipType = errors.New("clip type must be one of: clip, video, screenshot, gif")

	// ErrInvalidGIFWidth is returned when a GIF width is outside the form bounds.
	errInvalidGIFWidth = fmt.Errorf("gif width must be between %d and %d", gifMinWidth, gifMaxWidth)
	// ErrInvalidGIFFPS is returned when a GIF fps is outside the form bounds.
	errInvalidGIFFPS = fmt.Errorf("gif fps must be between %d and %d", gifMinFPS, gifMaxFPS)
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
	}
}

// Cancel stops a pending or processing clip.
func (handler *ClipHandler) Cancel(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return respond.WriteError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	if !handler.clipQueue.Cancel(id) {
		return respond.WriteError(ctx, fiber.StatusConflict, "not_cancellable", "clip is not running")
	}

	updated := handler.clipQueue.GetJob(id)
	if updated != nil {
		err := handler.db.SaveClip(ctx.Context(), updated)
		if err != nil {
			return respond.WriteError(ctx, fiber.StatusInternalServerError, respond.PersistFailed, err.Error())
		}

		job = updated
	}

	if respond.IsHTMXRequest(ctx) {
		ctx.Set("HX-Refresh", "true")

		return respond.SendStatusCode(ctx, fiber.StatusOK)
	}

	if respond.IsFormRequest(ctx) {
		return respond.RedirectTo(ctx, respond.ClipReturnPath(job.MediaID))
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, clipResponse(job))
}

// Create handles the create clip request.
func (handler *ClipHandler) Create(ctx fiber.Ctx) error {
	req, err := parseClipRequest(ctx)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, respond.InvalidRequest, err.Error())
	}

	jobType, ok := sharedclip.NormalizeClipType(req.ClipType)
	if !ok {
		return respond.WriteError(
			ctx,
			fiber.StatusBadRequest,
			"invalid_clip_type",
			errInvalidClipType.Error(),
		)
	}

	err = handler.validateClipParams(jobType, req)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, respond.InvalidRequest, err.Error())
	}

	req.Quality, err = handler.resolveQuality(ctx.Context(), req.Quality)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, "invalid_quality", err.Error())
	}

	inputPath, err := handler.resolveInput(ctx.Context(), req.MediaID)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, "media_path", err.Error())
	}

	job := buildJob(&req, jobType, inputPath)
	sharedclip.AssignOutputPaths(job, handler.clipStorage)
	sharedclip.ApplyDefaults(job)

	err = handler.db.SaveClip(ctx.Context(), job)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusInternalServerError, respond.PersistFailed, err.Error())
	}

	handler.clipQueue.Submit(job)

	if respond.IsFormRequest(ctx) {
		return respond.RedirectTo(ctx, respond.ClipReturnPath(req.MediaID))
	}

	return respond.WriteJSON(ctx, fiber.StatusCreated, clipResponse(job))
}

// Delete handles the delete clip request.
func (handler *ClipHandler) Delete(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return respond.WriteError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	if job.OutputPath != "" {
		err := handler.clipStorage.DeleteFile(job.OutputPath)
		if err != nil {
			return respond.WriteError(ctx, fiber.StatusInternalServerError, "delete_failed", err.Error())
		}
	}

	err := handler.db.DeleteClip(ctx.Context(), id)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusInternalServerError, "delete_failed", err.Error())
	}

	handler.clipQueue.Delete(id)

	err = ctx.SendStatus(fiber.StatusNoContent)
	if err != nil {
		return fmt.Errorf("send status: %w", err)
	}

	return nil
}

// Download handles the download clip request.
func (handler *ClipHandler) Download(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	job := handler.lookupJob(ctx.Context(), id)
	if job == nil {
		return respond.WriteError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	if job.Status != queue.JobStatusCompleted {
		return respond.WriteError(ctx, fiber.StatusConflict, "not_ready", "clip is not ready for download")
	}

	if !handler.clipStorage.FileExists(job.OutputPath) {
		return respond.WriteError(
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
		return respond.WriteError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, clipResponse(job))
}

// List handles the list clips request.
func (handler *ClipHandler) List(ctx fiber.Ctx) error {
	jobs := handler.listJobs(ctx.Context())
	clips := make([]ClipResponse, 0, len(jobs))

	for _, job := range jobs {
		clips = append(clips, clipResponse(job))
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, fiber.Map{"clips": clips})
}

// Preview renders a short low-quality segment without saving a clip.
func (handler *ClipHandler) Preview(ctx fiber.Ctx) error {
	req, err := parseClipRequest(ctx)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, respond.InvalidRequest, err.Error())
	}

	inputPath, err := handler.resolveInput(ctx.Context(), req.MediaID)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, "media_path", err.Error())
	}

	previewID := uuid.New().String()
	output := handler.clipStorage.PreviewPath(previewID)
	ffmpeg := media.NewExecFFmpeg(handler.cfg.FFmpegPath, handler.cfg.FFprobePath)

	crop := media.CropRect{}
	if req.CropBlackBars {
		detected, detectErr := ffmpeg.DetectCrop(
			ctx.Context(),
			inputPath,
			req.StartTime,
			req.Duration,
		)
		if detectErr == nil {
			crop = detected
		}
	}

	err = ffmpeg.ExtractPreview(
		ctx.Context(),
		inputPath,
		output,
		req.StartTime,
		req.Duration,
		req.AudioIndex,
		crop,
		media.QualityPreset{WebSafeColor: derefBool(req.WebSafeColor)},
	)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusInternalServerError, "preview_failed", err.Error())
	}

	err = handler.clipStorage.Put(ctx.Context(), output)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusInternalServerError, "preview_failed", err.Error())
	}

	end := req.StartTime + req.Duration

	return respond.RedirectTo(ctx, "/media/item/"+req.MediaID+
		"?preview="+previewID+
		"&start="+strconv.FormatFloat(req.StartTime, 'f', 1, 64)+
		"&end="+strconv.FormatFloat(end, 'f', 1, 64)+
		"&"+respond.QueryWebSafeColor+"="+webSafeQueryValue(req.WebSafeColor))
}

// Update saves clip metadata and optionally regenerates the file.
func (handler *ClipHandler) Update(ctx fiber.Ctx) error {
	job := handler.lookupJob(ctx.Context(), ctx.Params(paramID))
	if job == nil {
		return respond.WriteError(ctx, fiber.StatusNotFound, errorNotFound, messageNotFound)
	}

	req, err := parseClipRequest(ctx)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, respond.InvalidRequest, err.Error())
	}

	err = handler.applyRequestQuality(ctx.Context(), &req)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, "invalid_quality", err.Error())
	}

	jobType, err := clipJobType(req.ClipType, job.Type)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, "invalid_clip_type", err.Error())
	}

	err = handler.validateClipParams(jobType, req)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadRequest, respond.InvalidRequest, err.Error())
	}

	applyClipEdits(job, req)

	err = handler.db.SaveClip(ctx.Context(), job)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusInternalServerError, respond.PersistFailed, err.Error())
	}

	err = handler.maybeRegenerate(ctx, job)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusInternalServerError, respond.PersistFailed, err.Error())
	}

	return respond.RedirectTo(ctx, respond.ClipReturnPath(job.MediaID))
}

// applyClipEdits writes editable clip fields onto a stored job.
func applyClipEdits(job *queue.Job, req ClipRequest) {
	jobType, ok := sharedclip.NormalizeClipType(req.ClipType)
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

// applyRequestQuality resolves a non-empty quality field onto a profile id.
func (handler *ClipHandler) applyRequestQuality(ctx context.Context, req *ClipRequest) error {
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

	err := handler.queueRegenerate(ctx.Context(), job)
	if err != nil {
		return fmt.Errorf("regenerate: %w", err)
	}

	return nil
}

// queueRegenerate re-queues a clip after metadata changes.
func (handler *ClipHandler) queueRegenerate(ctx context.Context, job *queue.Job) error {
	sharedclip.AssignOutputPaths(job, handler.clipStorage)

	job.Status = queue.JobStatusPending
	job.Progress = 0
	job.Error = ""

	err := handler.db.SaveClip(ctx, job)
	if err != nil {
		return fmt.Errorf("save regenerate: %w", err)
	}

	handler.clipQueue.Submit(job)

	return nil
}

// resolveInput maps a media id onto a local filesystem path.
func (handler *ClipHandler) resolveInput(ctx context.Context, mediaID string) (string, error) {
	path, err := ResolveMediaPath(
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
func ResolveMediaPath(
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
func (handler *ClipHandler) validateClipParams(jobType queue.JobType, req ClipRequest) error {
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
		maxDur = respond.DefaultMaxClipDur
	}

	if duration <= 0 || duration > float64(maxDur) {
		return fmt.Errorf("%w: must be between 0 and %d seconds", errInvalidDuration, maxDur)
	}

	return nil
}

// clipJobType prefers the requested clip type, then the stored job type.
func clipJobType(clipType string, fallback queue.JobType) (queue.JobType, error) {
	if clipType == "" {
		return fallback, nil
	}

	jobType, ok := sharedclip.NormalizeClipType(clipType)
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
func parseClipRequest(ctx fiber.Ctx) (ClipRequest, error) {
	if strings.Contains(ctx.Get(fiber.HeaderContentType), "json") {
		var req ClipRequest

		err := ctx.Bind().Body(&req)
		if err != nil {
			return ClipRequest{}, fmt.Errorf("bind json: %w", err)
		}

		return req, nil
	}

	start := formSeconds(ctx, "startTime")
	duration := formSeconds(ctx, "duration")
	if duration == 0 {
		end := formSeconds(ctx, "endTime")
		if end > start {
			duration = end - start
		}
	}

	return ClipRequest{
		Name:          ctx.FormValue("name"),
		MediaID:       ctx.FormValue("mediaId"),
		MediaTitle:    ctx.FormValue("mediaTitle"),
		MediaType:     ctx.FormValue("mediaType"),
		StartTime:     start,
		Duration:      duration,
		Quality:       ctx.FormValue("quality"),
		ClipType:      ctx.FormValue("clipType"),
		Width:         formInt(ctx, "width"),
		FPS:           formInt(ctx, "fps"),
		AudioIndex:    formInt(ctx, "audioIndex"),
		CropBlackBars: ctx.FormValue("cropBlackBars") == formChecked,
		WebSafeColor:  new(ctx.FormValue("webSafeColor") == formChecked),
	}, nil
}

// derefBool returns the pointed value, or false when ptr is nil.
//
// Parameters:
//   - value: Optional boolean from JSON or form binding.
//
// Returns:
//   - result: *value when set, otherwise false.
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
//   - raw: formChecked when true, otherwise respond.QueryUnchecked.
func webSafeQueryValue(value *bool) string {
	if derefBool(value) {
		return formChecked
	}

	return respond.QueryUnchecked
}

// formInt parses a form field as int, or 0.
func formInt(ctx fiber.Ctx, name string) int {
	value, err := strconv.Atoi(ctx.FormValue(name))
	if err != nil {
		return 0
	}

	return value
}

// formSeconds parses a form field as a timecode or raw seconds.
func formSeconds(ctx fiber.Ctx, name string) float64 {
	tc, err := media.Parse(ctx.FormValue(name))
	if err != nil {
		return 0
	}

	return tc.Seconds()
}

// clipName prefers the user-supplied name, then the media title.
func clipName(req *ClipRequest) string {
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

// buildJob constructs a pending queue job from a clip request.
func buildJob(req *ClipRequest, jobType queue.JobType, inputPath string) *queue.Job {
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
		Status:        queue.JobStatusPending,
		Progress:      0,
		Error:         "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// clipResponse maps a job onto the public clip payload.
func clipResponse(job *queue.Job) ClipResponse {
	return ClipResponse{
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
	}
}
