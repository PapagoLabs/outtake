// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/exportform"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// mediaLoadFailedMsg is shown when Plex metadata cannot be loaded for a media item.
const mediaLoadFailedMsg = "Couldn't load this title from Plex. " +
	"A clip can still be made if Outtake can read the file."

// MediaItem renders a single media item with a player and its clips.
//
// Parameters:
//   - ctx: Incoming page request.
//
// Returns:
//   - err: Non-nil when the page cannot be rendered.
func (handler *Handler) MediaItem(ctx fiber.Ctx) error {
	id := ctx.Params(routes.ParamID)
	item, itemErr := handler.loadMediaItem(ctx, id)
	query := parseClipListQuery(ctx)
	source := handler.describeItem(ctx, id, item, itemErr)
	audience := handler.audience(ctx)
	clips, moreURL := handler.clipsForMedia(ctx, id, source, query, audience)
	window := exportform.ClipWindow(ctx)

	props := pages.MediaItemPageProps{
		ID:           id,
		Title:        id,
		Type:         "",
		MaxDur:       clip.DurationCap(handler.cfg.MaxClipDur),
		Clips:        clips,
		ClipsMoreURL: moreURL,
		SharedClips:  handler.sharedClipsForMedia(ctx, id, source, audience),
		ClipStatus:   query.Status,
		ClipType:     query.Type,
		ClipQuery:    query.Query,
		ClipSort:     query.Sort,
		Profiles:     profile.SelectableProfiles(ctx.Context(), handler.db),
		AudioTracks:  view.AudioTrackOptions(source.AudioStreams),
		Error:        mediaItemError(ctx, itemErr),
		PreviewID:    ctx.Query(routes.QueryPreview),
		StartTime:    window.Start,
		EndTime:      window.End,
		Crumbs:       nil,
		LibraryID:    "",
		Quality:      source.Quality,
		SourceHDR:    source.HDR,
	}
	if itemErr == nil {
		props.Title = item.DisplayTitle()
		props.Type = item.Type
		props.Duration = timecode.FromSeconds(item.Duration).Duration()
		props.LibraryID = item.LibraryID
		props.Crumbs = view.ItemCrumbs(item, handler.sidebarLibraries(ctx))
	}

	// The export form falls back to the media title, so it is resolved last.
	props.Export = exportform.MediaItemForm(ctx, handler.cfg, props.Title)

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.MediaItemPage(props).Render(ctx.Context(), writer)
	})
}

// MediaItemClips renders the media-item clip list fragment for HTMX swaps,
// or the next page a scrolled list appends.
//
// Parameters:
//   - ctx: Request with optional clip filters, sort, and page.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) MediaItemClips(ctx fiber.Ctx) error {
	id := ctx.Params(routes.ParamID)
	query := parseClipListQuery(ctx)
	clips, moreURL := handler.clipsForMedia(
		ctx,
		id,
		handler.sources.Describe(ctx.Context(), id),
		query,
		handler.audience(ctx),
	)

	if respond.TargetsElement(ctx, routes.TargetItemClipsMore) {
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return pages.ItemClipsMore(clips, moreURL).Render(ctx.Context(), writer)
		})
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.ItemClipList(clips, query.Filtered(), moreURL).Render(ctx.Context(), writer)
	})
}

// NewClip sends clip-now links to the media item editor.
//
// Parameters:
//   - ctx: Request carrying the media id and an optional start mark.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (*Handler) NewClip(ctx fiber.Ctx) error {
	mediaID := ctx.Query("mediaId")
	if mediaID == "" {
		return respond.RedirectTo(ctx, routes.PathMedia)
	}

	values := url.Values{}
	if start := ctx.Query(routes.QueryStart); start != "" {
		values.Set(routes.QueryStart, start)
	}

	return respond.RedirectTo(ctx, routes.ItemURL(mediaID, values))
}

// audience returns who a request's clip cards are shown to.
//
// Parameters:
//   - ctx: Request whose caller views the cards.
//
// Returns:
//   - audience: The caller's scope and the installation's usernames, which
//     are nil when they cannot be read.
func (handler *Handler) audience(ctx fiber.Ctx) view.Audience {
	names, err := handler.db.Usernames(ctx.Context())

	return view.NewAudience(middleware.ClipScope(ctx), names, err)
}

// clipsForMedia returns clip cards for one media id, stamped with source information.
//
// Parameters:
//   - ctx: Incoming page request.
//   - mediaID: Plex media item id.
//   - source: Probed source information for the media file.
//   - query: Normalized clip list filters and sort.
//   - audience: Who the cards are shown to.
//
// Returns:
//   - clips: The page's clip cards, carrying the probed audio, HDR flag, and
//     duration.
//   - moreURL: Loads the next page, empty when no clip follows.
func (handler *Handler) clipsForMedia(
	ctx fiber.Ctx,
	mediaID string,
	source library.SourceInfo,
	query catalog.ClipListQuery,
	audience view.Audience,
) ([]view.ClipItem, string) {
	page := catalog.Window(
		catalog.Apply(
			catalog.ForMedia(ctx.Context(), handler.db, audience.Scope, mediaID),
			query,
		),
		query,
	)

	path := routes.ItemURL(mediaID, nil) + routes.PathClips

	return handler.clipCards(ctx, page.Jobs, source, audience),
		view.MoreClipsURL(path, query, page)
}

// sharedClipsForMedia returns cards for other accounts' clips of one media id
// that are shared with the caller, newest first.
//
// Parameters:
//   - ctx: Incoming page request.
//   - mediaID: Plex media item id.
//   - source: Probed source information for the media file.
//   - audience: Who the cards are shown to.
//
// Returns:
//   - clips: Read-only clip cards, empty when none is shared with the caller.
func (handler *Handler) sharedClipsForMedia(
	ctx fiber.Ctx,
	mediaID string,
	source library.SourceInfo,
	audience view.Audience,
) []view.ClipItem {
	jobs := catalog.Apply(
		catalog.SharedForMedia(
			ctx.Context(),
			handler.db,
			audience.Scope,
			catalog.SeenBy(handler.sources, middleware.PlexViewer(ctx)),
			mediaID,
		),
		catalog.Normalize(catalog.ClipListQuery{}),
	)

	return handler.clipCards(ctx, jobs, source, audience)
}

// clipCards builds the cards for clips of one source.
//
// Parameters:
//   - ctx: Incoming page request.
//   - jobs: The clips, in the order shown.
//   - source: Probed source information for the media file.
//   - audience: Who the cards are shown to.
//
// Returns:
//   - clips: Clip cards carrying the probed audio, HDR flag, and duration.
func (handler *Handler) clipCards(
	ctx fiber.Ctx,
	jobs []*clip.Job,
	source library.SourceInfo,
	audience view.Audience,
) []view.ClipItem {
	clips := view.NewClipItems(
		jobs,
		profile.SelectableProfiles(ctx.Context(), handler.db),
		clip.DurationCap(handler.cfg.MaxClipDur),
		blob.ExistsEach(
			ctx.Context(),
			handler.outputs.Exists,
			append(clip.OutputPaths(jobs), clip.SDRPaths(jobs)...),
			blob.ExistsLimit,
		),
	)

	for index := range clips {
		view.ApplySource(&clips[index], source)
	}

	audience.ApplyEach(clips, jobs)

	return clips
}

// describeItem reports what the file behind a media page holds.
//
// The page already read the item's metadata, so a loaded item is described from
// the file path it carries rather than by asking Plex for that metadata again.
//
// Parameters:
//   - ctx: Request context.
//   - mediaID: Plex media item id.
//   - item: The metadata the page loaded.
//   - itemErr: Why the metadata could not be loaded, or nil.
//
// Returns:
//   - source: What the file turned out to be, zero when it could not be probed.
func (handler *Handler) describeItem(
	ctx fiber.Ctx,
	mediaID string,
	item plex.MediaItem,
	itemErr error,
) library.SourceInfo {
	owner := middleware.PlexViewer(ctx).Owner

	// Only the owner's lookup is retried, because an account the item is not
	// shared with may learn nothing about its file.
	if itemErr != nil {
		if !owner {
			return library.SourceInfo{}
		}

		return handler.sources.Describe(ctx.Context(), mediaID)
	}

	// A shared account's view of an item may leave out its file, which the
	// bound token still finds.
	if item.FilePath == "" {
		if owner {
			return library.SourceInfo{}
		}

		return handler.sources.Describe(ctx.Context(), mediaID)
	}

	return handler.sources.DescribePath(ctx.Context(), handler.cfg.RemapMediaPath(item.FilePath))
}

// loadMediaItem fetches metadata for a Plex rating key.
//
// Parameters:
//   - ctx: Request context.
//   - mediaID: Plex media item id.
//
// Returns:
//   - item: Media item metadata.
//   - err: Non-nil when no server is bound or the PMS lookup fails.
func (handler *Handler) loadMediaItem(ctx fiber.Ctx, mediaID string) (plex.MediaItem, error) {
	plexClient, server, ok := handler.plexPair(ctx)
	if !ok {
		return plex.MediaItem{}, library.ErrNoServer
	}

	item, err := plexClient.GetMediaItem(ctx.Context(), server, mediaID)
	if err != nil {
		return plex.MediaItem{}, fmt.Errorf("load media item: %w", err)
	}

	return *item, nil
}

// parseClipListQuery reads the clip filters and page carried on a media item
// page.
//
// Parameters:
//   - ctx: Request with optional type, q, sort, status, after, and offset
//     query parameters.
//
// Returns:
//   - query: Normalized list filters, sort, and page.
func parseClipListQuery(ctx fiber.Ctx) catalog.ClipListQuery {
	offset, err := strconv.Atoi(ctx.Query(routes.QueryClipOffset))
	if err != nil {
		offset = 0
	}

	return catalog.Normalize(catalog.ClipListQuery{
		Status: clip.Status(ctx.Query(routes.QueryClipStatus)),
		Type:   clip.Type(ctx.Query(routes.QueryClipType)),
		Query:  strings.TrimSpace(ctx.Query(routes.QueryClipName)),
		Sort:   ctx.Query(routes.QueryClipSort),
		After:  ctx.Query(routes.QueryClipAfter),
		Offset: offset,
	})
}

// mediaItemError is the notice the page shows when Plex metadata cannot be
// loaded.
//
// A form failure the session carries to the page outranks it, and the layout's
// banner shows that one instead.
//
// Parameters:
//   - ctx: The page request, whose session may hold a form failure.
//   - itemErr: Error from the Plex metadata load, which may be nil.
//
// Returns:
//   - message: The load-failure notice, or an empty string.
func mediaItemError(ctx fiber.Ctx, itemErr error) string {
	if itemErr == nil || respond.HasFlash(ctx) {
		return ""
	}

	return mediaLoadFailedMsg
}
