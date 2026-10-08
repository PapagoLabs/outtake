// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"fmt"
	"io"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	clipcard "github.com/PapagoLabs/outtake/internal/web/components/clip"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

const (
	// queryType is the clip type filter query parameter.
	queryType = "type"
	// queryQ is the clip name search query parameter.
	queryQ = "q"
	// querySort is the clip list sort query parameter.
	querySort = "sort"

	// describeLimit is how many clip sources the clips page probes at once.
	describeLimit = 4
)

// ClipFile streams a completed clip for in-browser playback.
//
// Parameters:
//   - ctx: Request for one clip id.
//
// Returns:
//   - err: Non-nil when the clip is missing or the stream fails.
func (handler *Handler) ClipFile(ctx fiber.Ctx) error {
	job := catalog.Job(ctx.Context(), handler.clipQueue, handler.db, ctx.Params(routes.ParamID))
	if job == nil || job.Status != clipdom.StatusCompleted || job.OutputPath == "" {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	err := handler.clipStorage.Ensure(ctx.Context(), job.OutputPath)
	if err != nil {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	err = respond.SendRangedFile(ctx, job.OutputPath)
	if err != nil {
		return fmt.Errorf("send clip file: %w", err)
	}

	return nil
}

// ClipRow renders the live status region of one clip card for HTMX polling.
//
// Parameters:
//   - ctx: Poll request for one clip id.
//
// Returns:
//   - err: Non-nil when the clip is missing or rendering fails.
func (handler *Handler) ClipRow(ctx fiber.Ctx) error {
	job := catalog.Job(ctx.Context(), handler.clipQueue, handler.db, ctx.Params(routes.ParamID))
	if job == nil {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		item := view.NewClipItem(
			job,
			profile.SelectableProfiles(ctx.Context(), handler.db),
			clipdom.DurationCap(handler.cfg.MaxClipDur),
			handler.outputExists(ctx.Context())(job.OutputPath),
		)

		return clipcard.ClipStatus(item).Render(ctx.Context(), writer)
	})
}

// Clips renders the clips list page, or the list fragment HTMX swaps into it.
//
// Parameters:
//   - ctx: Request with optional clip filters and sort.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) Clips(ctx fiber.Ctx) error {
	query := parseClipListQuery(ctx)
	jobs := catalog.Apply(catalog.Jobs(ctx.Context(), handler.clipQueue, handler.db), query)
	items := view.NewClipItems(
		jobs,
		profile.SelectableProfiles(ctx.Context(), handler.db),
		clipdom.DurationCap(handler.cfg.MaxClipDur),
		blob.ExistsEach(
			ctx.Context(),
			handler.clipStorage.Exists,
			clipdom.OutputPaths(jobs),
			blob.ExistsLimit,
		),
	)

	// Each card shows the choices its own source allows, such as the HDR
	// checkbox, so the sources are described the way the media page does.
	sources := handler.sources.DescribePaths(ctx.Context(), clipdom.InputPaths(jobs), describeLimit)
	for index, job := range jobs {
		view.ApplySource(&items[index], sources[job.InputPath])
	}

	props := pages.ClipsProps{
		Items:  items,
		Status: query.Status,
		Type:   query.Type,
		Query:  query.Query,
		Sort:   query.Sort,
	}

	if wantsClipList(ctx) {
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return pages.ClipsList(props).Render(ctx.Context(), writer)
		})
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.Clips(props).Render(ctx.Context(), writer)
	})
}

// wantsClipList reports whether the request should swap the clips list only.
//
// Parameters:
//   - ctx: Request context with an optional HX-Target header.
//
// Returns:
//   - True when HTMX is targeting #clip-list.
func wantsClipList(ctx fiber.Ctx) bool {
	return respond.TargetsElement(ctx, routes.TargetClipList)
}

// parseClipListQuery reads type, name, sort, and status from the request.
//
// Parameters:
//   - ctx: Request with optional type, q, sort, and status query parameters.
//
// Returns:
//   - query: Normalized list filters and sort.
func parseClipListQuery(ctx fiber.Ctx) catalog.ClipListQuery {
	return catalog.Normalize(catalog.ClipListQuery{
		Status: clipdom.Status(ctx.Query("status")),
		Type:   clipdom.Type(ctx.Query(queryType)),
		Query:  strings.TrimSpace(ctx.Query(queryQ)),
		Sort:   ctx.Query(querySort),
	})
}
