// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"time"

	"github.com/PapagoLabs/outtake/internal/plex/library"
)

// LibraryItem is a Plex library in the sidebar and media browse grid.
type LibraryItem struct {
	ID        string
	Title     string
	Type      string
	ThumbPath string
}

// MediaItem is one title or container in the media library grid.
type MediaItem struct {
	ID           string
	Title        string
	Type         string
	Duration     time.Duration
	ThumbPath    string
	Browsable    bool
	BrowseURL    string
	Year         int
	Index        int
	ParentIndex  int
	ShowTitle    string
	EpisodeLabel string
	TitleSort    string
	AddedAt      int64
}

// Crumb is one step in the media library trail.
type Crumb struct {
	Title string
	URL   string
}

// LetterIndex is one first-character jump target on a library section.
type LetterIndex = library.LetterIndex

// MediaProps is the media library page and its HTMX results fragment.
type MediaProps struct {
	Items       []MediaItem
	Libraries   []LibraryItem
	Crumbs      []Crumb
	Letters     []LetterIndex
	Query       string
	LibraryID   string
	ParentID    string
	ParentTitle string
	UpID        string
	UpTitle     string
	Sort        string
	Letter      string
	Start       int
	Total       int
	PageSize    int
	HasServer   bool
}

const (
	// MediaSortTitleAsc lists titles A-Z.
	MediaSortTitleAsc = library.SortTitleAsc
	// MediaSortTitleDesc lists titles Z-A.
	MediaSortTitleDesc = library.SortTitleDesc
	// MediaSortYearDesc lists newest years first.
	MediaSortYearDesc = library.SortYearDesc
	// MediaSortYearAsc lists oldest years first.
	MediaSortYearAsc = library.SortYearAsc
	// MediaSortAddedDesc lists recently added titles first.
	MediaSortAddedDesc = library.SortAddedDesc
	// MediaSortAddedAsc lists oldest added titles first.
	MediaSortAddedAsc = library.SortAddedAsc

	// JumpOtherKey is the mark for a jump target that is neither a letter, a year, nor a month.
	JumpOtherKey = library.JumpOtherKey
)

// SortSelected reports whether a sort option is the active one.
//
// Parameters:
//   - current: Active sort key, or empty when the query carried none.
//   - value: Sort key the option being rendered stands for.
//   - fallback: Sort key shown when current is empty.
//
// Returns:
//   - selected: True when the option is the active one.
func SortSelected(current, value, fallback string) bool {
	if current == "" {
		return value == fallback
	}

	return current == value
}

// TitleJumpKey returns the first-letter jump key for a title.
//
// Parameters:
//   - titleSort: PMS titleSort value, preferred when set.
//   - title: Display title used when titleSort is empty.
//
// Returns:
//   - key: A through Z, or JumpOtherKey for anything else.
func TitleJumpKey(titleSort, title string) string {
	return library.TitleJumpKey(titleSort, title)
}
