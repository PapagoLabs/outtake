// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// SortTitleAsc lists titles A-Z.
	SortTitleAsc = "title_asc"
	// SortTitleDesc lists titles Z-A.
	SortTitleDesc = "title_desc"
	// SortYearDesc lists newest years first.
	SortYearDesc = "year_desc"
	// SortYearAsc lists oldest years first.
	SortYearAsc = "year_asc"
	// SortAddedDesc lists recently added titles first.
	SortAddedDesc = "added_desc"
	// SortAddedAsc lists oldest added titles first.
	SortAddedAsc = "added_asc"

	// JumpOtherKey is the mark for a jump target that is neither a letter, a year, nor a month.
	JumpOtherKey = "#"
)

// TitleJumpKey returns the first-letter jump key for a title.
//
// Parameters:
//   - titleSort: PMS titleSort value, preferred when set.
//   - title: Display title used when titleSort is empty.
//
// Returns:
//   - key: A through Z, or JumpOtherKey for anything else.
func TitleJumpKey(titleSort, title string) string {
	key := strings.TrimSpace(titleSort)
	if key == "" {
		key = strings.TrimSpace(title)
	}

	if key == "" {
		return JumpOtherKey
	}

	// Decoding the leading rune and converting the whole string agree on the
	// first character, and both yield U+FFFD when the leading bytes are not
	// valid UTF-8, which falls out of the A-Z range the same way either way.
	first, _ := utf8.DecodeRuneInString(key)

	upper := unicode.ToUpper(first)
	if upper >= 'A' && upper <= 'Z' {
		return string(upper)
	}

	return JumpOtherKey
}

// PlexSort maps an Outtake sort key onto a PMS sort value.
//
// Parameters:
//   - sortKey: Normalized Outtake sort key.
//
// Returns:
//   - plexSort: PMS sort query value, or empty when the key is unset.
func PlexSort(sortKey string) string {
	switch sortKey {
	case SortTitleAsc:
		return "titleSort:asc"
	case SortTitleDesc:
		return "titleSort:desc"
	case SortYearDesc:
		return "year:desc"
	case SortYearAsc:
		return "year:asc"
	case SortAddedDesc:
		return "addedAt:desc"
	case SortAddedAsc:
		return "addedAt:asc"
	default:
		return ""
	}
}
