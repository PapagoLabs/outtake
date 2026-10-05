// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/plex"
)

func TestParseStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want int
	}{
		{name: "absent", give: "", want: 0},
		{name: "offset", give: "48", want: 48},
		{name: "zero", give: "0", want: 0},
		{name: "negative", give: "-1", want: 0},
		{name: "malformed", give: "abc", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, ParseStart(test.give))
		})
	}
}

func TestNormalizeQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give Query
		want Query
	}{
		{
			name: "empty query",
			give: Query{},
			want: Query{},
		},
		{
			name: "library falls back to title sort",
			give: Query{LibraryID: "1"},
			want: Query{LibraryID: "1", Sort: SortTitleAsc},
		},
		{
			name: "known sort is kept",
			give: Query{LibraryID: "1", Sort: SortYearDesc},
			want: Query{LibraryID: "1", Sort: SortYearDesc},
		},
		{
			name: "unknown sort falls back",
			give: Query{LibraryID: "1", Sort: "bogus"},
			want: Query{LibraryID: "1", Sort: SortTitleAsc},
		},
		{
			name: "unknown sort without library is dropped",
			give: Query{Sort: "bogus"},
			want: Query{},
		},
		{
			name: "letter survives on a library root",
			give: Query{LibraryID: "1", Letter: "M"},
			want: Query{LibraryID: "1", Sort: SortTitleAsc, Letter: "M"},
		},
		{
			name: "letter and start together",
			give: Query{LibraryID: "1", Sort: SortTitleAsc, Letter: "M", Start: 48},
			want: Query{LibraryID: "1", Sort: SortTitleAsc, Letter: "M", Start: 48},
		},
		{
			name: "nested container drops sort and letter",
			give: Query{LibraryID: "1", ParentID: "9", Sort: SortTitleAsc, Letter: "A", Start: 48},
			want: Query{LibraryID: "1", ParentID: "9", Start: 48},
		},
		{
			name: "search drops letter",
			give: Query{Query: "star wars", LibraryID: "1", Sort: SortYearDesc, Letter: "M"},
			want: Query{Query: "star wars", LibraryID: "1", Sort: SortYearDesc},
		},
		{
			name: "search drops before",
			give: Query{Query: "star wars", LibraryID: "1", Sort: SortTitleAsc, Before: 43},
			want: Query{Query: "star wars", LibraryID: "1", Sort: SortTitleAsc},
		},
		{
			name: "before survives on a library root",
			give: Query{LibraryID: "1", Sort: SortTitleAsc, Before: 43},
			want: Query{LibraryID: "1", Sort: SortTitleAsc, Before: 43},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, NormalizeQuery(test.give))
		})
	}
}

func TestQueryListStart(t *testing.T) {
	t.Parallel()

	buckets := []plex.LetterIndex{
		{Title: "#", Size: 3},
		{Title: "A", Size: 40},
		{Title: "M", Size: 8},
	}

	tests := []struct {
		name  string
		query Query
		want  int
	}{
		{
			name:  "letter without start",
			query: Query{Letter: "M"},
			want:  43,
		},
		{
			name:  "start wins over letter",
			query: Query{Letter: "M", Start: 48},
			want:  48,
		},
		{
			name:  "zero start without letter",
			query: Query{},
			want:  0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.query.ListStart(buckets))
		})
	}
}

func TestQueryWindow(t *testing.T) {
	t.Parallel()

	buckets := []plex.LetterIndex{
		{Title: "#", Size: 3},
		{Title: "A", Size: 40},
	}

	tests := []struct {
		name  string
		query Query
		want  Window
	}{
		{
			name:  "before below one page",
			query: Query{Before: 43},
			want:  Window{Start: 0, Size: 43},
		},
		{
			name:  "before above one page",
			query: Query{Before: 96},
			want:  Window{Start: 48, Size: 48},
		},
		{
			name:  "letter without before",
			query: Query{Letter: "A"},
			want:  Window{Start: 3, Size: PageSize},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.query.Window(buckets))
		})
	}
}

func TestQueryShowJumpIndex(t *testing.T) {
	t.Parallel()

	assert.True(t, Query{LibraryID: "1"}.ShowJumpIndex())
	assert.False(t, Query{}.ShowJumpIndex())
	assert.False(t, Query{LibraryID: "1", ParentID: "9"}.ShowJumpIndex())
	assert.False(t, Query{LibraryID: "1", Query: "movie"}.ShowJumpIndex())
}
