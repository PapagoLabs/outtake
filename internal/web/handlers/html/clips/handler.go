// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clips

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/components/clip"
	clipapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/clip"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	htmldeps "github.com/PapagoLabs/outtake/internal/web/handlers/html/deps"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	"github.com/PapagoLabs/outtake/internal/web/pages/clips"
)

// Handler serves clips HTML routes.
type Handler struct {
	rt *htmldeps.Runtime
}

// New constructs a clips HTML handler.
//
// Parameters:
//   - rt: Rt.
//
// Returns:
//   - handler: A clips HTML handler.
func New(rt *htmldeps.Runtime) *Handler {
	return &Handler{rt: rt}
}

// ClipFile handles the HTTP request.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (h *Handler) ClipFile(ctx fiber.Ctx) error {
	job := h.rt.LookupClip(ctx, ctx.Params(htmldeps.ParamID))
	if job == nil || job.Status != queue.JobStatusCompleted || job.OutputPath == "" {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	if !htmldeps.ClipFileExists(job.OutputPath) {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	err := respond.SendRangedFile(ctx, job.OutputPath)
	if err != nil {
		return fmt.Errorf("send clip file: %w", err)
	}

	return nil
}

// ClipRow handles the HTTP request.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (h *Handler) ClipRow(ctx fiber.Ctx) error {
	job := h.rt.LookupClip(ctx, ctx.Params(htmldeps.ParamID))
	if job == nil {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		item := htmldeps.ToClipItem(job, h.rt.ClipProfileOptions(ctx), h.rt.ClipMaxDur())

		return clip.ClipCard(item).Render(ctx.Context(), writer)
	})
}

// Clips handles the HTTP request.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (h *Handler) Clips(ctx fiber.Ctx) error {
	query := clipapi.ParseListQuery(ctx)
	props := clips.ClipsProps{
		Items:  h.rt.JobsToClipItems(ctx, clipapi.ApplyListQuery(h.rt.ListJobs(ctx), query)),
		Status: query.Status,
		Type:   query.Type,
		Query:  query.Query,
		Sort:   query.Sort,
	}

	if htmldeps.WantsClipList(ctx) {
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return clips.ClipsList(props).Render(ctx.Context(), writer)
		})
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return clips.Clips(props).Render(ctx.Context(), writer)
	})
}

// NewClip handles the HTTP request.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (*Handler) NewClip(ctx fiber.Ctx) error {
	mediaID := ctx.Query("mediaId")
	if mediaID == "" {
		return respond.RedirectTo(ctx, respond.PathMedia)
	}

	values := url.Values{}
	if start := ctx.Query(respond.QueryStart); start != "" {
		values.Set(respond.QueryStart, start)
	}

	return respond.RedirectTo(ctx, htmldeps.MediaItemLocation(mediaID, values))
}

// PreviewFile handles the HTTP request.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (h *Handler) PreviewFile(ctx fiber.Ctx) error {
	id := ctx.Params(htmldeps.ParamID)
	path := filepath.Join(h.rt.Cfg.StoragePath, "previews", id+".mp4")

	err := respond.SendRangedFile(ctx, path)
	if err != nil {
		return fmt.Errorf("send preview: %w", err)
	}

	return nil
}
