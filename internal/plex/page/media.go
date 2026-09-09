// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package page

import (
	"slices"
	"strconv"
	"strings"
)

// MediaItem represents a media item in Plex.
type MediaItem struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Type             string  `json:"type"`
	Duration         float64 `json:"duration"`
	ThumbPath        string  `json:"thumbPath"`
	LibraryTitle     string  `json:"libraryTitle"`
	LibraryID        string  `json:"libraryId,omitempty"`
	Year             int     `json:"year,omitempty"`
	Index            int     `json:"index,omitempty"`
	ParentIndex      int     `json:"parentIndex,omitempty"`
	ParentID         string  `json:"parentId,omitempty"`
	ParentTitle      string  `json:"parentTitle,omitempty"`
	GrandparentID    string  `json:"grandparentId,omitempty"`
	GrandparentTitle string  `json:"grandparentTitle,omitempty"`
	TitleSort        string  `json:"titleSort,omitempty"`
	AddedAt          int64   `json:"addedAt,omitempty"`
}

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
func ReverseIndexes(index []LetterIndex) []LetterIndex {
	out := make([]LetterIndex, 0, len(index))
	for _, entry := range slices.Backward(index) {
		out = append(out, entry)
	}

	return out
}

// SortYearIndexes orders year buckets from oldest to newest.
func SortYearIndexes(index []LetterIndex) []LetterIndex {
	out := append([]LetterIndex(nil), index...)
	slices.SortFunc(out, compareYearTitles)

	return out
}

func compareYearTitles(left, right LetterIndex) int {
	leftYear, leftErr := strconv.Atoi(left.Title)
	rightYear, rightErr := strconv.Atoi(right.Title)
	if leftErr != nil || rightErr != nil {
		return strings.Compare(left.Title, right.Title)
	}

	return leftYear - rightYear
}
