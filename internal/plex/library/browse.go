// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// Browsed is what one media library page request resolved to.
type Browsed struct {
	// Libraries are the libraries the selected server offers.
	Libraries []plex.Library
	// Items are the media items of the requested window, absent for the library
	// chooser and for a search that matched nothing.
	Items []plex.MediaItem
	// Total is how many items the listing matched.
	Total int
}

const (
	// yearFacet is the PMS directory facet that buckets a section by year.
	yearFacet = "year"

	// firstCharacterFacet is the PMS directory facet that buckets by letter.
	firstCharacterFacet = "firstCharacter"
)

// errNoAddedAtCache reports a caller that asked for the added-at jump rail
// without the cache that rail is built through.
var errNoAddedAtCache = errors.New("added-at jump index needs its cache")

// Browse lists the libraries of a server together with the media of one
// requested window, which is either a search, a container listing, a library
// section listing, or the library chooser itself.
//
// Parameters:
//   - ctx: Request context.
//   - client: PMS client.
//   - server: PMS to query.
//   - query: Normalized browse state.
//   - window: Container offset and page size the listing covers.
//
// Returns:
//   - browsed: The libraries and the items of the window.
//   - err: Non-nil when Plex could not be asked, and the libraries are absent.
func Browse(
	ctx context.Context,
	client *plex.Client,
	server plex.Server,
	query Query,
	window Window,
) (Browsed, error) {
	libraries, err := client.GetLibraries(ctx, server)
	if err != nil {
		return Browsed{}, fmt.Errorf("list libraries: %w", err)
	}

	browsed := Browsed{Libraries: libraries}

	if query.Query != "" {
		found, searchErr := client.SearchOnServer(ctx, server, query.Query, query.LibraryID)
		if searchErr != nil {
			return browsed, fmt.Errorf("search media: %w", searchErr)
		}

		browsed.Items = found
		browsed.Total = len(found)

		return browsed, nil
	}

	if query.LibraryID == "" {
		return browsed, nil
	}

	page, err := browsePage(ctx, client, server, query, window)
	if err != nil {
		return browsed, fmt.Errorf("browse section: %w", err)
	}

	browsed.Items = page.Items
	browsed.Total = page.Total

	return browsed, nil
}

// browsePage loads one page of a library section or of container children.
//
// Parameters:
//   - ctx: Request context.
//   - client: PMS client.
//   - server: PMS to query.
//   - query: Normalized browse state.
//   - window: Container offset and page size.
//
// Returns:
//   - page: Items and total size for the requested window.
//   - err: Non-nil when the PMS request fails.
func browsePage(
	ctx context.Context,
	client *plex.Client,
	server plex.Server,
	query Query,
	window Window,
) (plex.MediaPage, error) {
	if query.ParentID != "" {
		page, err := client.GetChildrenPage(ctx, server, query.ParentID, window.Start, window.Size)
		if err != nil {
			return plex.MediaPage{}, fmt.Errorf("list children: %w", err)
		}

		return page, nil
	}

	page, err := client.GetMediaPage(
		ctx,
		server,
		query.LibraryID,
		window.Start,
		window.Size,
		PlexSort(query.Sort),
	)
	if err != nil {
		return plex.MediaPage{}, fmt.Errorf("list section: %w", err)
	}

	return page, nil
}

// JumpIndex collects the jump-rail buckets a library section listing shows,
// which are letters, years, or added-at months depending on the sort.
//
// Parameters:
//   - ctx: Request context.
//   - client: PMS client.
//   - server: PMS to query.
//   - query: Normalized browse state.
//   - cache: Cache the added-at buckets are collected through, which may be nil
//     for a browse that cannot reach the added-at sorts.
//
// Returns:
//   - index: Title, year, or added-at buckets, nil when the browse has no rail.
//   - err: Non-nil when the buckets could not be collected.
func JumpIndex(
	ctx context.Context,
	client *plex.Client,
	server plex.Server,
	query Query,
	cache *AddedAtCache,
) ([]plex.LetterIndex, error) {
	if !query.ShowJumpIndex() {
		return nil, nil
	}

	if isAddedAtSort(query.Sort) {
		if cache == nil {
			return nil, errNoAddedAtCache
		}

		return cache.Load(ctx, client, server, query.LibraryID, query.Sort), nil
	}

	name := jumpFacet(query.Sort)

	buckets, err := client.GetSectionIndex(ctx, server, query.LibraryID, name)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", name, err)
	}

	return OrderIndex(buckets, query.Sort), nil
}

// isAddedAtSort reports whether a sort collects added-at buckets.
//
// Parameters:
//   - sort: Normalized sort key.
//
// Returns:
//   - True when the sort buckets by when an item was added.
func isAddedAtSort(sort string) bool {
	return sort == SortAddedDesc || sort == SortAddedAsc
}

// jumpFacet is the PMS directory facet a sort buckets by.
//
// Parameters:
//   - sort: Normalized sort key.
//
// Returns:
//   - years for a year sort, otherwise first characters.
func jumpFacet(sort string) string {
	if sort == SortYearDesc || sort == SortYearAsc {
		return yearFacet
	}

	return firstCharacterFacet
}
