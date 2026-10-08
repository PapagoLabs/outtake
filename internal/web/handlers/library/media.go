// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
	"io"

	"github.com/rs/zerolog/log"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/web/components/browse"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

const (
	// queryType is the clip type filter on a media item's clip list.
	queryType = "type"

	// queryQ is the search query parameter.
	queryQ = "q"

	// querySort is the sort query parameter.
	querySort = "sort"

	// queryLetter is the media library first-character jump parameter.
	queryLetter = "letter"

	// queryBefore is the exclusive end offset when prepending a previous page.
	queryBefore = "before"
)

// Media handles the media library page request.
//
// Parameters:
//   - ctx: Request with library browse query and optional HX-Target.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) Media(ctx fiber.Ctx) error {
	query := parseMediaListQuery(ctx)
	props := handler.mediaPageProps(ctx, query)

	err := renderMediaPage(ctx, &props)
	if err != nil {
		return fmt.Errorf("render media: %w", err)
	}

	return nil
}

// mediaPageProps builds library browse props for the current request.
//
// Parameters:
//   - ctx: Request with library browse query.
//   - query: Normalized browse state.
//
// Returns:
//   - props: Media library page model.
func (handler *Handler) mediaPageProps(
	ctx fiber.Ctx,
	query library.Query,
) view.MediaProps {
	plexClient, server, hasServer := handler.plexPair()

	var letters []plex.LetterIndex

	if hasServer && !wantsMediaMore(ctx) && !wantsMediaPrev(ctx) {
		letters = handler.mediaLetters(ctx, plexClient, server, query)
	}

	window := query.Window(letters)
	browsed := handler.browse(ctx, plexClient, server, query, window)
	libraries := view.LibraryItems(browsed.Libraries)

	return view.MediaProps{
		Items: view.MediaItems(
			browsed.Items,
			query.LibraryID,
			query.ParentID,
			ctx.Query(routes.QueryTitle),
		),
		Libraries: chooserLibraries(libraries, query.Query, query.LibraryID),
		Crumbs: view.MediaCrumbs(
			libraries,
			query.LibraryID,
			ctx.Query(routes.QueryUp),
			ctx.Query(routes.QueryUpTitle),
			ctx.Query(routes.QueryTitle),
		),
		Letters:     library.JumpMarks(letters, query.Sort),
		Query:       query.Query,
		LibraryID:   query.LibraryID,
		ParentID:    query.ParentID,
		ParentTitle: ctx.Query(routes.QueryTitle),
		UpID:        ctx.Query(routes.QueryUp),
		UpTitle:     ctx.Query(routes.QueryUpTitle),
		Sort:        query.Sort,
		Letter:      query.Letter,
		Start:       window.Start,
		Total:       browsed.Total,
		PageSize:    library.PageSize,
		HasServer:   hasServer,
	}
}

// browse resolves one media page's libraries and items, leaving the page empty
// when Plex cannot be reached.
//
// Parameters:
//   - ctx: Request context.
//   - plexClient: PMS client, nil when no server is bound.
//   - server: PMS to query.
//   - query: Normalized browse state.
//   - window: Container offset and page size.
//
// Returns:
//   - browsed: The libraries and the items of the window.
func (handler *Handler) browse(
	ctx fiber.Ctx,
	plexClient *plex.Client,
	server plex.Server,
	query library.Query,
	window library.Window,
) library.Browsed {
	if plexClient == nil {
		return library.Browsed{}
	}

	browsed, err := library.Browse(
		ctx.Context(),
		&handler.libraries,
		plexClient,
		server,
		query,
		window,
	)
	if err != nil {
		log.Warn().Err(err).Msg("browse media failed")
	}

	return browsed
}

// mediaLetters collects the jump-rail buckets for a library section listing.
//
// Parameters:
//   - ctx: Request context.
//   - plexClient: PMS client, nil when no server is bound.
//   - server: PMS to query.
//   - query: Normalized browse state.
//
// Returns:
//   - letters: Title, year, or added-at buckets, nil when the listing has no rail.
func (handler *Handler) mediaLetters(
	ctx fiber.Ctx,
	plexClient *plex.Client,
	server plex.Server,
	query library.Query,
) []plex.LetterIndex {
	if plexClient == nil {
		return nil
	}

	letters, err := library.JumpIndex(
		ctx.Context(),
		plexClient,
		server,
		query,
		&handler.addedAt,
	)
	if err != nil {
		log.Warn().Err(err).Msg("collect jump index failed")
	}

	return letters
}

// chooserLibraries returns library cards only for the root media view.
//
// Parameters:
//   - libraries: Discovered libraries.
//   - query: Active search text.
//   - libraryID: Library the browse view is scoped to.
//
// Returns:
//   - items: The library cards, or nil once a search or library is active.
func chooserLibraries(libraries []view.LibraryItem, query, libraryID string) []view.LibraryItem {
	if query != "" || libraryID != "" {
		return nil
	}

	return libraries
}

// parseMediaListQuery reads search, library, parent, sort, letter, start, and before.
//
// Parameters:
//   - ctx: Request with optional q, library, parent, sort, letter, start, and before.
//
// Returns:
//   - query: Normalized browse state.
func parseMediaListQuery(ctx fiber.Ctx) library.Query {
	return library.NormalizeQuery(library.Query{
		Query:     ctx.Query(queryQ),
		LibraryID: ctx.Query(routes.QueryLibrary),
		ParentID:  ctx.Query(routes.QueryParent),
		Sort:      ctx.Query(querySort),
		Letter:    ctx.Query(queryLetter),
		Start:     library.ParseStart(ctx.Query(routes.QueryStart)),
		Before:    library.ParseStart(ctx.Query(queryBefore)),
	})
}

// renderMediaPage writes the media library page or an HTMX fragment.
//
// Parameters:
//   - ctx: Request with an optional HX-Target.
//   - props: Media library page model.
//
// Returns:
//   - err: Non-nil when rendering fails.
func renderMediaPage(ctx fiber.Ctx, props *view.MediaProps) error {
	switch {
	case wantsMediaPrev(ctx):
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaPrev(*props).Render(ctx.Context(), writer)
		})
	case wantsMediaMore(ctx):
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaMore(*props).Render(ctx.Context(), writer)
		})
	case wantsMediaResults(ctx):
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaBrowse(*props).Render(ctx.Context(), writer)
		})
	default:
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return pages.Media(*props).Render(ctx.Context(), writer)
		})
	}
}

// wantsMediaMore reports whether the request should append the next poster page.
//
// Parameters:
//   - ctx: Request context with an optional HX-Target header.
//
// Returns:
//   - ok: True when HTMX is targeting #media-more.
func wantsMediaMore(ctx fiber.Ctx) bool {
	return respond.TargetsElement(ctx, routes.TargetMediaMore)
}

// wantsMediaPrev reports whether the request should prepend the previous poster page.
//
// Parameters:
//   - ctx: Request context with an optional HX-Target header.
//
// Returns:
//   - ok: True when HTMX is targeting #media-prev.
func wantsMediaPrev(ctx fiber.Ctx) bool {
	return respond.TargetsElement(ctx, routes.TargetMediaPrev)
}

// wantsMediaResults reports whether the request should swap the media browse pane.
//
// Parameters:
//   - ctx: Request context with an optional HX-Target header.
//
// Returns:
//   - ok: True when HTMX is targeting #media-browse.
func wantsMediaResults(ctx fiber.Ctx) bool {
	return respond.TargetsElement(ctx, routes.TargetMediaBrowse)
}
