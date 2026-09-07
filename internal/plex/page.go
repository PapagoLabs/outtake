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
	out := make([]LetterIndex, 0, len(index))
	for _, entry := range slices.Backward(index) {
		out = append(out, entry)
	}

	return out
}

// SortYearIndexes orders year buckets from oldest to newest.
//
// Parameters:
//   - index: Year buckets in any order.
//
// Returns:
//   - index: A sorted copy.
func SortYearIndexes(index []LetterIndex) []LetterIndex {
	out := append([]LetterIndex(nil), index...)
	slices.SortFunc(out, compareYearTitles)

	return out
}

// compareYearTitles orders two year buckets numerically, oldest first.
//
// Parameters:
//   - left: First year bucket.
//   - right: Second year bucket.
//
// Returns:
//   - cmp: Negative when left is older, positive when newer.
func compareYearTitles(left, right LetterIndex) int {
	leftYear, leftErr := strconv.Atoi(left.Title)
	rightYear, rightErr := strconv.Atoi(right.Title)
	if leftErr != nil || rightErr != nil {
		return strings.Compare(left.Title, right.Title)
	}

	return leftYear - rightYear
}
