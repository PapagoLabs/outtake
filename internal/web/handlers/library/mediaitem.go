// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
	"io"
	"net/url"
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
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// mediaLoadFailedMsg is shown when Plex metadata cannot be loaded for a media item.
const mediaLoadFailedMsg = "Could not load this item from Plex. " +
	"You can still create a clip if the file is reachable."

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
	clips := handler.clipsForMedia(ctx, id, source, query)
	window := exportform.ClipWindow(ctx)

	props := pages.MediaItemPageProps{
		ID:          id,
		Title:       id,
		Type:        "",
		MaxDur:      clip.DurationCap(handler.cfg.MaxClipDur),
		Clips:       clips,
		ClipStatus:  query.Status,
		ClipType:    query.Type,
		ClipQuery:   query.Query,
		ClipSort:    query.Sort,
		Profiles:    profile.SelectableProfiles(ctx.Context(), handler.db),
		AudioTracks: view.AudioTrackOptions(source.AudioStreams),
		Error:       mediaItemError(itemErr, ctx.Query(routes.QueryError)),
		PreviewID:   ctx.Query(routes.QueryPreview),
		StartTime:   window.Start,
		EndTime:     window.End,
		Crumbs:      nil,
		LibraryID:   "",
		Quality:     source.Quality,
		SourceHDR:   source.HDR,
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

// MediaItemClips renders the media-item clip list fragment for HTMX swaps.
//
// Parameters:
//   - ctx: Request with optional clip filters and sort.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) MediaItemClips(ctx fiber.Ctx) error {
	id := ctx.Params(routes.ParamID)
	query := parseClipListQuery(ctx)
	clips := handler.clipsForMedia(
		ctx,
		id,
		handler.sources.Describe(ctx.Context(), id),
		query,
	)

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.ItemClipList(clips, query.Filtered()).Render(ctx.Context(), writer)
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

// clipsForMedia returns clip cards for one media id, stamped with source information.
//
// Parameters:
//   - ctx: Incoming page request.
//   - mediaID: Plex media item id.
//   - source: Probed source information for the media file.
//   - query: Normalized clip list filters and sort.
//
// Returns:
//   - clips: Clip cards carrying the probed audio, HDR flag, and duration.
func (handler *Handler) clipsForMedia(
	ctx fiber.Ctx,
	mediaID string,
	source library.SourceInfo,
	query catalog.ClipListQuery,
) []view.ClipItem {
	jobs := catalog.Apply(catalog.ForMedia(ctx.Context(), handler.db, mediaID), query)
	clips := view.NewClipItems(
		jobs,
		profile.SelectableProfiles(ctx.Context(), handler.db),
		clip.DurationCap(handler.cfg.MaxClipDur),
		blob.ExistsEach(
			ctx.Context(),
			handler.outputs.Exists,
			clip.OutputPaths(jobs),
			blob.ExistsLimit,
		),
	)

	for index := range clips {
		view.ApplySource(&clips[index], source)
	}

	return clips
}

// describeItem reports what the file behind a media page holds. The page
// already read the item's metadata, so a loaded item is described from the
// file path it carries rather than by asking Plex for that metadata again.
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
	if itemErr != nil {
		return handler.sources.Describe(ctx.Context(), mediaID)
	}

	if item.FilePath == "" {
		return library.SourceInfo{}
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
	plexClient, server, ok := handler.plexPair()
	if !ok {
		return plex.MediaItem{}, library.ErrNoServer
	}

	item, err := plexClient.GetMediaItem(ctx.Context(), server, mediaID)
	if err != nil {
		return plex.MediaItem{}, fmt.Errorf("load media item: %w", err)
	}

	return *item, nil
}

// parseClipListQuery reads the clip filters carried on a media item page.
//
// Parameters:
//   - ctx: Request with optional type, q, sort, and status query parameters.
//
// Returns:
//   - query: Normalized list filters and sort.
func parseClipListQuery(ctx fiber.Ctx) catalog.ClipListQuery {
	return catalog.Normalize(catalog.ClipListQuery{
		Status: clip.Status(ctx.Query("status")),
		Type:   clip.Type(ctx.Query(queryType)),
		Query:  strings.TrimSpace(ctx.Query(queryQ)),
		Sort:   ctx.Query(querySort),
	})
}

// mediaItemError prefers a form-flash query over a Plex metadata load failure.
//
// Parameters:
//   - itemErr: Error from the Plex metadata load, which may be nil.
//   - queryErr: Flash text carried in the request query.
//
// Returns:
//   - message: The flash text, the load-failure notice, or an empty string.
func mediaItemError(itemErr error, queryErr string) string {
	if queryErr != "" {
		return queryErr
	}

	if itemErr != nil {
		return mediaLoadFailedMsg
	}

	return ""
}
