// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	clipcard "github.com/PapagoLabs/outtake/internal/web/components/clip"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// describeLimit is how many clip sources the clips page probes at once.
const describeLimit = 4

// ClipFile streams a completed clip for in-browser playback.
//
// Parameters:
//   - ctx: Request for one clip id.
//
// Returns:
//   - err: Non-nil when the clip is missing or the stream fails.
func (handler *Handler) ClipFile(ctx fiber.Ctx) error {
	job := handler.viewJob(ctx.Context(), middleware.ViewpointOf(ctx), ctx.Params(routes.ParamID))
	if job == nil || job.Status != clipdom.StatusCompleted {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	//nolint:wrapcheck // The helper names what it was sending.
	return handler.sendClipFile(ctx, job.OutputPath)
}

// ClipSDRFile streams the SDR version of a completed HDR clip, which its card
// plays on a screen or in a browser that cannot show the clip's own file.
//
// Parameters:
//   - ctx: Request for one clip id.
//
// Returns:
//   - err: Non-nil when the clip or its SDR version is missing, or the stream
//     fails.
func (handler *Handler) ClipSDRFile(ctx fiber.Ctx) error {
	job := handler.viewJob(ctx.Context(), middleware.ViewpointOf(ctx), ctx.Params(routes.ParamID))
	if job == nil || job.Status != clipdom.StatusCompleted || !job.MayHaveSDRVersion() {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	//nolint:wrapcheck // The helper names what it was sending.
	return handler.sendClipFile(ctx, job.SDRPath())
}

// sendClipFile streams one of a clip's files, fetching it from storage first
// when only another instance has it.
//
// Parameters:
//   - ctx: The request.
//   - path: The file to send, empty when the clip has none.
//
// Returns:
//   - err: Non-nil when the stream fails.
func (handler *Handler) sendClipFile(ctx fiber.Ctx, path string) error {
	if path == "" {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	err := handler.clipStorage.Ensure(ctx.Context(), path)
	if err != nil {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	err = respond.SendRangedFile(ctx, path)
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
	job := handler.viewJob(ctx.Context(), middleware.ViewpointOf(ctx), ctx.Params(routes.ParamID))
	if job == nil {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		item := handler.statusItem(
			job,
			profile.SelectableProfiles(ctx.Context(), handler.db),
			handler.storedFiles(ctx.Context(), job),
			handler.audience(ctx),
		)

		return clipcard.ClipStatus(item).Render(ctx.Context(), writer)
	})
}

// statusItem builds the card model a clip's status region renders from.
//
// Parameters:
//   - job: The clip.
//   - profiles: Profiles the card offers as a quality choice.
//   - files: Which of the clip's files are stored.
//   - audience: Who the card is shown to.
//
// Returns:
//   - item: The card model.
func (handler *Handler) statusItem(
	job *clipdom.Job,
	profiles []profile.ProfileOption,
	files storedFiles,
	audience view.Audience,
) view.ClipItem {
	item := view.NewClipItem(
		job,
		profiles,
		clipdom.DurationCap(handler.cfg.MaxClipDur),
		files.output,
	)

	item.SDRExists = files.sdr

	audience.Apply(&item, job.OwnerID)

	return item
}

// storedFiles asks the storage which of a clip's files it holds.
//
// Parameters:
//   - ctx: Request scope for the storage lookups.
//   - job: The clip.
//
// Returns:
//   - files: Which of the clip's files are stored.
func (handler *Handler) storedFiles(ctx context.Context, job *clipdom.Job) storedFiles {
	return storedFiles{
		output: handler.outputExists(ctx)(job.OutputPath),
		sdr:    handler.sdrVersionExists(ctx, job),
	}
}

// Clips renders the clips list page, the list fragment HTMX swaps into it, or
// the next page a scrolled list appends.
//
// A page holds catalog.PageSize cards, and only its own clips are checked in
// storage and probed.
//
// Parameters:
//   - ctx: Request with optional clip filters, sort, and page.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) Clips(ctx fiber.Ctx) error {
	query := parseClipListQuery(ctx)
	page := catalog.Window(catalog.Apply(handler.listedFor(ctx, query), query), query)
	jobs := page.Jobs
	items := view.NewClipItems(
		jobs,
		profile.SelectableProfiles(ctx.Context(), handler.db),
		clipdom.DurationCap(handler.cfg.MaxClipDur),
		blob.ExistsEach(
			ctx.Context(),
			handler.clipStorage.Exists,
			append(clipdom.OutputPaths(jobs), clipdom.SDRPaths(jobs)...),
			blob.ExistsLimit,
		),
	)

	// Each card shows the choices its own source allows, such as the HDR
	// checkbox, so the sources are described the way the media page does.
	sources := handler.sources.DescribePaths(ctx.Context(), clipdom.InputPaths(jobs), describeLimit)
	for index, job := range jobs {
		view.ApplySource(&items[index], sources[job.InputPath])
	}

	audience := handler.audience(ctx)
	audience.ApplyEach(items, jobs)

	props := pages.ClipsProps{
		Items:       items,
		Status:      query.Status,
		Type:        query.Type,
		Query:       query.Query,
		Sort:        query.Sort,
		Shared:      query.Shared,
		OfferShared: audience.Shares(),
		MoreURL:     view.MoreClipsURL(routes.PathClips, query, page),
		Bulk:        handler.bulkBar(ctx, query, items, audience),
	}

	if respond.TargetsElement(ctx, routes.TargetClipListMore) {
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return pages.ClipsMore(props).Render(ctx.Context(), writer)
		})
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

// bulkBar describes the clips page's bulk actions.
//
// Retry Failed counts the caller's own failed clips, so the Shared view, which
// lists other accounts' clips too, does not offer it.
//
// Parameters:
//   - ctx: Request whose caller the clips are kept to.
//   - query: Normalized toolbar state.
//   - items: The cards the page shows.
//   - audience: Who the cards are shown to.
//
// Returns:
//   - props: The bar.
func (handler *Handler) bulkBar(
	ctx fiber.Ctx,
	query catalog.ClipListQuery,
	items []view.ClipItem,
	audience view.Audience,
) clipcard.BulkBarProps {
	failed := 0
	if !query.Shared {
		failed = len(handler.failedJobs(ctx, ""))
	}

	return clipcard.BulkBarProps{
		ListID:     routes.TargetClipList,
		ToolbarID:  routes.ClipListToolbar,
		StatusID:   routes.ClipBulkStatus,
		MediaID:    "",
		Failed:     failed,
		Selectable: view.AnySelectable(items),
		CanShare:   audience.Shares(),
	}
}

// listedFor returns the clips a list draws from before its filters.
//
// Parameters:
//   - ctx: Request whose caller the clips are kept to.
//   - query: Normalized toolbar state.
//
// Returns:
//   - jobs: The shared clips the caller sees for the Shared view, and every
//     clip the caller may change otherwise.
func (handler *Handler) listedFor(ctx fiber.Ctx, query catalog.ClipListQuery) []*clipdom.Job {
	if !query.Shared {
		return handler.listJobs(ctx)
	}

	viewpoint := middleware.ViewpointOf(ctx)

	return catalog.Shared(
		ctx.Context(),
		handler.clipQueue,
		handler.db,
		viewpoint.Scope,
		catalog.SeenBy(handler.sources, viewpoint.Viewer),
	)
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

// parseClipListQuery reads the filters, sort, page, and which clips to show
// from the request.
//
// Parameters:
//   - ctx: Request with optional type, q, sort, status, show, after, and
//     offset query parameters.
//
// Returns:
//   - query: Normalized list filters, sort, and page.
func parseClipListQuery(ctx fiber.Ctx) catalog.ClipListQuery {
	offset, err := strconv.Atoi(ctx.Query(routes.QueryClipOffset))
	if err != nil {
		offset = 0
	}

	return catalog.Normalize(catalog.ClipListQuery{
		Status: clipdom.Status(ctx.Query(routes.QueryClipStatus)),
		Type:   clipdom.Type(ctx.Query(routes.QueryClipType)),
		Query:  strings.TrimSpace(ctx.Query(routes.QueryClipName)),
		Sort:   ctx.Query(routes.QueryClipSort),
		After:  ctx.Query(routes.QueryClipAfter),
		Offset: offset,
		Shared: ctx.Query(routes.QueryShow) == routes.ShowShared,
	})
}
