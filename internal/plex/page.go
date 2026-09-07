// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"slices"
	"strconv"
	"strings"
)

// MediaPage is one page of library or container children.
type MediaPage struct {
	Items []MediaItem
	Total int
	Start int
	Size  int
}

// LetterIndex is one first-character bucket from a library section.
type LetterIndex struct {
	Title string
	Size  int
}

// LetterOffset returns the item offset of letter in index order.
//
// Parameters:
//   - index: First-character buckets in PMS order.
//   - letter: Directory title to find, such as "A" or "#".
//
// Returns:
//   - offset: Sum of sizes before the matching title, or 0 when missing.
func LetterOffset(index []LetterIndex, letter string) int {
	offset := 0

	for _, entry := range index {
		if entry.Title == letter {
			return offset
		}

		offset += entry.Size
	}

	return 0
}

// ReverseIndexes returns index in reverse order.
//
// Parameters:
//   - index: Buckets in original order.
//
// Returns:
//   - index: A reversed copy.
func ReverseIndexes(index []LetterIndex) []LetterIndex {
	out := make([]LetterIndex, len(index))
	for i, entry := range index {
		out[len(index)-1-i] = entry
	}

	return out
}

// SortYearIndexes orders year buckets numerically.
//
// Parameters:
//   - index: Year buckets in any order.
//   - desc: True to sort newest years first.
//
// Returns:
//   - index: A sorted copy.
func SortYearIndexes(index []LetterIndex, desc bool) []LetterIndex {
	out := append([]LetterIndex(nil), index...)
	slices.SortFunc(out, func(a, b LetterIndex) int {
		ay, aErr := strconv.Atoi(a.Title)
		by, bErr := strconv.Atoi(b.Title)
		if aErr != nil || bErr != nil {
			if desc {
				return strings.Compare(b.Title, a.Title)
			}

			return strings.Compare(a.Title, b.Title)
		}

		if desc {
			return by - ay
		}

		return ay - by
	})

	return out
}
