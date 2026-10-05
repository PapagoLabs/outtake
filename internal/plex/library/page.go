// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"strconv"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// Query is the media library browse query state.
type Query struct {
	Query     string
	LibraryID string
	ParentID  string
	Sort      string
	Letter    string
	Start     int
	Before    int
}

// Window is the PMS container offset and page size for a query.
type Window struct {
	Start int
	Size  int
}

// PageSize is the number of posters shown per media library page.
const PageSize = 48

// ParseStart parses a non-negative pagination offset.
//
// Parameters:
//   - raw: Offset carried by the request, which may be absent or malformed.
//
// Returns:
//   - start: The offset, or 0 when it is not a non-negative integer.
func ParseStart(raw string) int {
	start, err := strconv.Atoi(raw)
	if err != nil || start < 0 {
		return 0
	}

	return start
}

// NormalizeQuery drops unknown sort values and nested letter jumps.
//
// Parameters:
//   - query: Raw browse query from the request.
//
// Returns:
//   - query: Normalized browse state.
func NormalizeQuery(query Query) Query {
	if query.ParentID != "" {
		query.Sort = ""
		query.Letter = ""
		query.Before = 0

		return query
	}

	switch query.Sort {
	case SortTitleAsc,
		SortTitleDesc,
		SortYearDesc,
		SortYearAsc,
		SortAddedDesc,
		SortAddedAsc:
	default:
		if query.LibraryID != "" {
			query.Sort = SortTitleAsc
		} else {
			query.Sort = ""
		}
	}

	if query.Query != "" {
		query.Letter = ""
		query.Before = 0
	}

	return query
}

// IsLibraryRoot reports whether the query is a section listing.
//
// Returns:
//   - ok: True when a library is selected without a parent or search.
func (query Query) IsLibraryRoot() bool {
	return query.LibraryID != "" && query.ParentID == "" && query.Query == ""
}

// ListStart returns the Plex container offset for this query.
//
// Parameters:
//   - buckets: First-character buckets used when letter is set and start is 0.
//
// Returns:
//   - start: Container offset.
func (query Query) ListStart(buckets []plex.LetterIndex) int {
	if query.Start > 0 {
		return query.Start
	}

	if query.Letter == "" {
		return 0
	}

	return plex.LetterOffset(buckets, query.Letter)
}

// ShowJumpIndex reports whether the library root has a jump rail.
//
// Returns:
//   - ok: True when the query is a library section listing.
func (query Query) ShowJumpIndex() bool {
	return query.IsLibraryRoot()
}

// Window returns the PMS offset and page size for this query.
//
// Parameters:
//   - buckets: First-character buckets used when letter is set and start is 0.
//
// Returns:
//   - window: Container offset and page size.
func (query Query) Window(buckets []plex.LetterIndex) Window {
	if query.Before > 0 {
		start := max(query.Before-PageSize, 0)

		return Window{Start: start, Size: query.Before - start}
	}

	return Window{Start: query.ListStart(buckets), Size: PageSize}
}
