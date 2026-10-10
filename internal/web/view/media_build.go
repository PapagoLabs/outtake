// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"net/url"
	"strconv"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

const (
	// crumbLibraries is the first crumb on every media trail.
	crumbLibraries = "Media Libraries"

	// seasonFallback is the crumb title for a season Plex gave no title for.
	seasonFallback = "Season"
)

// ThumbSrc rewrites a Plex thumb path onto the local cache proxy.
//
// Parameters:
//   - path: Plex thumb path, which may be empty or malformed.
//
// Returns:
//   - proxyURL: Local thumb proxy URL, or empty when the path cannot be proxied.
func ThumbSrc(path string) string {
	if path == "" || !plex.ValidThumbPath(path) {
		return ""
	}

	return routes.PathThumb + "?" + routes.QueryThumbPath + "=" + url.QueryEscape(path)
}

// LibraryItems maps Plex libraries onto page models.
//
// Parameters:
//   - libs: Plex libraries.
//
// Returns:
//   - items: Page models in the order Plex returned them.
func LibraryItems(libs []plex.Library) []LibraryItem {
	out := make([]LibraryItem, 0, len(libs))
	for _, lib := range libs {
		out = append(out, LibraryItem{
			ID:        lib.ID,
			Title:     lib.Title,
			Type:      lib.Type,
			ThumbPath: ThumbSrc(lib.ThumbPath),
		})
	}

	return out
}

// MediaItems maps Plex media onto page models.
//
// Parameters:
//   - items: Plex media items from a listing or a search.
//   - libraryID: Section key every card is filed under, which overrides the one
//     the item carries.
//   - parentID: Rating key of the container being drilled into.
//   - parentTitle: Title of that container, carried on the drill-down link.
//
// Returns:
//   - items: Page models in the order Plex returned them.
func MediaItems(
	items []plex.MediaItem,
	libraryID, parentID, parentTitle string,
) []MediaItem {
	out := make([]MediaItem, 0, len(items))

	for index := range items {
		item := items[index]

		libID := libraryID
		if libID == "" {
			libID = item.LibraryID
		}

		episodeLabel := ""
		if item.Type == plex.TypeEpisode {
			episodeLabel = plex.EpisodeCode(item.ParentIndex, item.Index)
		}

		out = append(out, MediaItem{
			ID:           item.ID,
			Title:        item.Title,
			Type:         item.Type,
			Duration:     timecode.FromSeconds(item.Duration).Duration(),
			ThumbPath:    ThumbSrc(item.ThumbPath),
			Browsable:    plex.IsContainerType(item.Type),
			BrowseURL:    routes.BrowseURL(libID, item.ID, item.Title, parentID, parentTitle),
			Year:         item.Year,
			Index:        item.Index,
			ParentIndex:  item.ParentIndex,
			ShowTitle:    item.GrandparentTitle,
			EpisodeLabel: episodeLabel,
			TitleSort:    item.TitleSort,
			AddedAt:      item.AddedAt,
		})
	}

	return out
}

// MediaCrumbs builds the library, container, and current title trail for a
// browse view.
//
// Parameters:
//   - libs: Discovered libraries, used to resolve the library title.
//   - libraryID: Library the browse view is scoped to.
//   - upID: Parent container id to link back to.
//   - upTitle: Parent container title.
//   - title: Current container title, shown without a link.
//
// Returns:
//   - crumbs: Trail from the library root down to the current container.
func MediaCrumbs(libs []LibraryItem, libraryID, upID, upTitle, title string) []Crumb {
	crumbs := []Crumb{{Title: crumbLibraries, URL: routes.PathMedia}}
	if libraryID == "" {
		return crumbs
	}

	libURL := routes.LibraryURL(libraryID)

	crumbs = append(crumbs, Crumb{Title: libraryTitle(libs, libraryID), URL: libURL})

	if upID != "" {
		crumbs = append(crumbs, Crumb{
			Title: upTitle,
			URL:   routes.BrowseURL(libraryID, upID, upTitle, "", ""),
		})
	}

	if title != "" {
		crumbs = append(crumbs, Crumb{Title: title})
	}

	return crumbs
}

// ItemCrumbs builds Libraries / library / show / season / title for a media item.
//
// Parameters:
//   - item: Media item being rendered.
//   - libs: Discovered libraries, used to resolve the library title.
//
// Returns:
//   - crumbs: The full trail, ending with the item's own title.
func ItemCrumbs(item plex.MediaItem, libs []LibraryItem) []Crumb {
	crumbs := []Crumb{{Title: crumbLibraries, URL: routes.PathMedia}}

	if item.LibraryID != "" {
		title := item.LibraryTitle
		if fromLibrary, ok := findLibrary(libs, item.LibraryID); ok {
			title = fromLibrary
		}

		if title == "" {
			title = item.LibraryID
		}

		crumbs = append(crumbs, Crumb{
			Title: title,
			URL:   routes.LibraryURL(item.LibraryID),
		})
	}

	crumbs = appendShowCrumbs(crumbs, item)

	return append(crumbs, Crumb{Title: item.Title})
}

// appendShowCrumbs adds show and season links for episodes.
//
// Parameters:
//   - crumbs: Trail built so far.
//   - item: Media item being rendered.
//
// Returns:
//   - crumbs: The trail, with the show and season crumbs that apply.
func appendShowCrumbs(crumbs []Crumb, item plex.MediaItem) []Crumb {
	if item.GrandparentID != "" && item.GrandparentTitle != "" {
		crumbs = append(crumbs, Crumb{
			Title: item.GrandparentTitle,
			URL: routes.BrowseURL(
				item.LibraryID,
				item.GrandparentID,
				item.GrandparentTitle,
				"",
				"",
			),
		})
	}

	if item.Type == plex.TypeSeason && item.ParentID != "" && item.ParentTitle != "" {
		return append(crumbs, Crumb{
			Title: item.ParentTitle,
			URL: routes.BrowseURL(
				item.LibraryID,
				item.ParentID,
				item.ParentTitle,
				"",
				"",
			),
		})
	}

	if item.Type != plex.TypeEpisode || item.ParentID == "" {
		return crumbs
	}

	seasonTitle := item.ParentTitle
	if seasonTitle == "" {
		seasonTitle = seasonFallback
	}

	return append(crumbs, Crumb{
		Title: seasonTitle,
		URL: routes.BrowseURL(
			item.LibraryID,
			item.ParentID,
			seasonTitle,
			item.GrandparentID,
			item.GrandparentTitle,
		),
	})
}

// SessionTitleParts builds dashboard title crumbs for a live session.
//
// Parameters:
//   - item: Media item the session is playing.
//
// Returns:
//   - parts: Title crumbs, which may be a single plain-text crumb.
//   - year: Release year, reported only for a non-episode.
func SessionTitleParts(item plex.MediaItem) ([]Crumb, int) {
	if item.Type != plex.TypeEpisode {
		if item.Title == "" {
			return displayTitleCrumb(item), 0
		}

		return []Crumb{{Title: item.Title, URL: SessionItemURL(item.ID)}}, item.Year
	}

	var parts []Crumb

	if item.GrandparentTitle != "" {
		parts = append(parts, Crumb{
			Title: item.GrandparentTitle,
			URL: SessionBrowseURL(
				item.LibraryID,
				item.GrandparentID,
				item.GrandparentTitle,
				"",
				"",
			),
		})
	}

	if label := SeasonLabel(item); label != "" {
		parts = append(parts, Crumb{
			Title: label,
			URL: SessionBrowseURL(
				item.LibraryID,
				item.ParentID,
				label,
				item.GrandparentID,
				item.GrandparentTitle,
			),
		})
	}

	if item.Title != "" {
		parts = append(parts, Crumb{Title: item.Title, URL: SessionItemURL(item.ID)})
	}

	if len(parts) == 0 {
		return displayTitleCrumb(item), 0
	}

	return parts, 0
}

// SeasonLabel returns ParentTitle, or Season N from ParentIndex.
//
// Parameters:
//   - item: Media item the session is playing.
//
// Returns:
//   - label: The parent title, a season name, or an empty string.
func SeasonLabel(item plex.MediaItem) string {
	if item.ParentTitle != "" {
		return item.ParentTitle
	}

	if item.ParentIndex > 0 {
		return seasonFallback + " " + strconv.Itoa(item.ParentIndex)
	}

	return ""
}

// SessionBrowseURL returns a container browse URL when library and item ids
// exist.
//
// Parameters:
//   - libraryID: Library the container belongs to.
//   - itemID: Container to browse.
//   - itemTitle: Container title for the link.
//   - parentID: Optional show id the container sits under.
//   - parentTitle: Optional show title for the link.
//
// Returns:
//   - location: Browse URL, or an empty string when an id is missing.
func SessionBrowseURL(libraryID, itemID, itemTitle, parentID, parentTitle string) string {
	if libraryID == "" || itemID == "" {
		return ""
	}

	return routes.BrowseURL(libraryID, itemID, itemTitle, parentID, parentTitle)
}

// SessionItemURL returns the media item path when id is set.
//
// Parameters:
//   - id: Plex media item id.
//
// Returns:
//   - location: Media item URL, or an empty string when the id is unset.
func SessionItemURL(id string) string {
	if id == "" {
		return ""
	}

	return routes.ItemURL(id, nil)
}

// displayTitleCrumb is a plain-text fallback when structured parts are missing.
//
// Parameters:
//   - item: Media item the session is playing.
//
// Returns:
//   - crumb: A single plain-text crumb, or nil when the item has no title.
func displayTitleCrumb(item plex.MediaItem) []Crumb {
	title := item.DisplayTitle()
	if title == "" {
		return nil
	}

	return []Crumb{{Title: title}}
}

// libraryTitle resolves a library id to its title, falling back to the id.
//
// Parameters:
//   - libs: Discovered libraries.
//   - libraryID: Library to resolve.
//
// Returns:
//   - title: The library title, or the id when no library carries it.
func libraryTitle(libs []LibraryItem, libraryID string) string {
	if title, ok := findLibrary(libs, libraryID); ok {
		return title
	}

	return libraryID
}

// findLibrary resolves a library id to its title.
//
// Parameters:
//   - libs: Discovered libraries.
//   - libraryID: Library to resolve.
//
// Returns:
//   - title: The library title.
//   - ok: False when no discovered library carries that id.
func findLibrary(libs []LibraryItem, libraryID string) (string, bool) {
	for _, lib := range libs {
		if lib.ID == libraryID {
			return lib.Title, true
		}
	}

	return "", false
}
