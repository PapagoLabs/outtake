// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTitleJumpKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		titleSort string
		title     string
		want      string
	}{
		{name: "title sort wins", titleSort: "Movie", title: "The Movie", want: "M"},
		{name: "title used when sort empty", title: "other", want: "O"},
		{name: "digit falls out of range", titleSort: "2 Movie", want: JumpOtherKey},
		{name: "empty title", title: "   ", want: JumpOtherKey},
		{name: "both empty", want: JumpOtherKey},
		{name: "unicode is not a letter", titleSort: "Árranc", want: JumpOtherKey},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, TitleJumpKey(test.titleSort, test.title))
		})
	}
}

func TestPlexSort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give string
		want string
	}{
		{give: SortTitleAsc, want: "titleSort:asc"},
		{give: SortTitleDesc, want: "titleSort:desc"},
		{give: SortYearDesc, want: "year:desc"},
		{give: SortYearAsc, want: "year:asc"},
		{give: SortAddedDesc, want: "addedAt:desc"},
		{give: SortAddedAsc, want: "addedAt:asc"},
		{give: "", want: ""},
		{give: "bogus", want: ""},
	}

	for _, test := range tests {
		t.Run(test.give, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, PlexSort(test.give))
		})
	}
}
