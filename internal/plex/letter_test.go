// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLetterOffset(t *testing.T) {
	t.Parallel()

	index := []LetterIndex{
		{Title: "#", Size: 3},
		{Title: "A", Size: 40},
		{Title: "B", Size: 12},
		{Title: "C", Size: 0},
		{Title: "M", Size: 8},
	}

	tests := []struct {
		give string
		want int
	}{
		{give: "#", want: 0},
		{give: "A", want: 3},
		{give: "B", want: 43},
		{give: "M", want: 55},
		{give: "Z", want: 0},
		{give: "", want: 0},
	}

	for _, test := range tests {
		t.Run(test.give, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, LetterOffset(index, test.give))
		})
	}
}

func TestReverseIndexes(t *testing.T) {
	t.Parallel()

	got := ReverseIndexes([]LetterIndex{
		{Title: "A", Size: 2},
		{Title: "B", Size: 3},
	})

	assert.Equal(t, []LetterIndex{
		{Title: "B", Size: 3},
		{Title: "A", Size: 2},
	}, got)
}

func TestSortYearIndexes(t *testing.T) {
	t.Parallel()

	index := []LetterIndex{
		{Title: "1995", Size: 2},
		{Title: "2024", Size: 1},
		{Title: "2001", Size: 4},
	}

	assert.Equal(t, []LetterIndex{
		{Title: "2024", Size: 1},
		{Title: "2001", Size: 4},
		{Title: "1995", Size: 2},
	}, SortYearIndexes(index, true))

	assert.Equal(t, []LetterIndex{
		{Title: "1995", Size: 2},
		{Title: "2001", Size: 4},
		{Title: "2024", Size: 1},
	}, SortYearIndexes(index, false))
}
