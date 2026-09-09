// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"fmt"
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"strconv"

	plextitle "github.com/PapagoLabs/outtake/internal/plex/title"
	"github.com/PapagoLabs/outtake/internal/web/components/nav"
	"github.com/PapagoLabs/outtake/internal/web/components/playback"
	clipapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/clip"

	htmldeps "github.com/PapagoLabs/outtake/internal/web/handlers/html/deps"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	mediapage "github.com/PapagoLabs/outtake/internal/web/pages/media"
	viewplayback "github.com/PapagoLabs/outtake/internal/web/view/playback"
)

// Handler serves media HTML routes.
type Handler struct {
	rt *htmldeps.Runtime
}

// New constructs a media HTML handler.
//
// Parameters:
//   - rt: Shared HTML handler runtime.
//
// Returns:
//   - handler: The media HTML handler.
func New(rt *htmldeps.Runtime) *Handler {
	return &Handler{rt: rt}
}

// Media renders the media library browse page.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: Non-nil when the media page cannot be rendered.
func (h *Handler) Media(ctx fiber.Ctx) error {
	query := htmldeps.ParseMediaListQuery(ctx)
	props := h.rt.MediaPageProps(ctx, query)

	err := htmldeps.RenderMediaPage(ctx, &props)
	if err != nil {
		return fmt.Errorf("render media: %w", err)
	}

	return nil
}

// MediaItem renders the media item detail and clip editor page.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: Non-nil when the media item HTML cannot be rendered.
func (h *Handler) MediaItem(ctx fiber.Ctx) error {
	id := ctx.Params(htmldeps.ParamID)
	item, itemErr := h.rt.LoadMediaItem(ctx, id)
	query := clipapi.ParseListQuery(ctx)
	tracks := h.rt.MediaAudioTracks(ctx, id)
	clips := h.rt.ClipsForMedia(ctx, id)

	for index := range clips {
		clips[index].AudioTracks = tracks
	}

	maxDur := h.rt.Cfg.MaxClipDurSec
	if maxDur <= 0 {
		maxDur = respond.DefaultMaxClipDur
	}

	start, err := strconv.ParseFloat(ctx.Query("start"), respond.FloatBitSize)
	if err != nil {
		start = 0
	}

	end, err := strconv.ParseFloat(ctx.Query("end"), respond.FloatBitSize)
	if err != nil || end <= start {
		end = start + respond.DefaultSegmentSecs
	}

	props := mediapage.MediaItemPageProps{
		ID:            id,
		Title:         id,
		Type:          "",
		Duration:      0,
		MaxDur:        maxDur,
		Clips:         clips,
		ClipStatus:    query.Status,
		ClipType:      query.Type,
		ClipQuery:     query.Query,
		ClipSort:      query.Sort,
		Profiles:      h.rt.ClipProfileOptions(ctx),
		AudioTracks:   tracks,
		Error:         respond.MediaItemError(itemErr, ctx.Query(respond.QueryError)),
		PreviewID:     ctx.Query("preview"),
		StartTime:     start,
		EndTime:       end,
		CropBlackBars: h.rt.Cfg.CropBlackBars,
		WebSafeColor:  htmldeps.PreviewWebSafeColor(ctx, h.rt.Cfg.WebSafeColor),
		Crumbs:        nil,
		LibraryID:     "",
	}
	if itemErr == nil {
		props.Title = plextitle.Display(item)
		props.Type = item.Type
		props.Duration = item.Duration
		props.LibraryID = item.LibraryID
		props.Crumbs = htmldeps.ItemCrumbs(item, h.rt.SidebarLibraries(ctx))
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return mediapage.MediaItemPage(props).Render(ctx.Context(), writer)
	})
}

// MediaItemClips renders the HTMX clip list fragment for a media item.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: Non-nil when the media-item clips HTML cannot be rendered.
func (h *Handler) MediaItemClips(ctx fiber.Ctx) error {
	id := ctx.Params(htmldeps.ParamID)
	query := clipapi.ParseListQuery(ctx)
	tracks := h.rt.MediaAudioTracks(ctx, id)
	clips := h.rt.ClipsForMedia(ctx, id)

	for index := range clips {
		clips[index].AudioTracks = tracks
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return mediapage.ItemClipList(clips, query.Filtered()).Render(ctx.Context(), writer)
	})
}

// NavLibraries renders the sidebar library list fragment.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: Non-nil when the libraries nav HTML cannot be rendered.
func (h *Handler) NavLibraries(ctx fiber.Ctx) error {
	selected := htmldeps.SelectedLibraryID(ctx.Get("HX-Current-URL"), ctx.Query(respond.QueryLibrary))

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return nav.NavLibraries(h.rt.SidebarLibraries(ctx), selected).
			Render(ctx.Context(), writer)
	})
}

// Playback renders the now-playing playback fragment for a media item.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: Non-nil when the playback HTML cannot be rendered.
func (h *Handler) Playback(ctx fiber.Ctx) error {
	mediaID := ctx.Params(htmldeps.ParamID)
	props := viewplayback.Playback{
		Playing:    false,
		ViewOffset: 0,
		Title:      "",
	}

	sessions := h.rt.Bind.Sessions()
	for index := range sessions {
		if sessions[index].MediaItem.ID != mediaID {
			continue
		}

		props.Playing = true
		props.ViewOffset = sessions[index].ViewOffset
		props.Title = sessions[index].Title

		break
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return playback.PlaybackPanel(props).Render(ctx.Context(), writer)
	})
}
