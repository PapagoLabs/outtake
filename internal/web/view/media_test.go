// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortSelected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		current, value, fallback string
		want                     bool
	}{
		{
			name:     "an empty current sort falls back to the default",
			current:  "",
			value:    MediaSortTitleAsc,
			fallback: MediaSortTitleAsc,
			want:     true,
		},
		{
			name:     "an empty current sort rejects every other option",
			current:  "",
			value:    MediaSortYearDesc,
			fallback: MediaSortTitleAsc,
			want:     false,
		},
		{
			name:     "the active sort is selected",
			current:  MediaSortYearDesc,
			value:    MediaSortYearDesc,
			fallback: MediaSortTitleAsc,
			want:     true,
		},
		{
			name:     "a set current sort is not resolved through the fallback",
			current:  MediaSortYearDesc,
			value:    MediaSortTitleAsc,
			fallback: MediaSortTitleAsc,
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, SortSelected(test.current, test.value, test.fallback))
		})
	}
}

func TestTitleJumpKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		giveTitleSort string
		giveTitle     string
		want          string
	}{
		{
			name:          "titleSort is preferred over the display title",
			giveTitleSort: "The Matrix",
			giveTitle:     "Ignored",
			want:          "T",
		},
		{
			name:      "an empty titleSort falls back to the title",
			giveTitle: "alien",
			want:      "A",
		},
		{
			name:          "a blank titleSort is still empty",
			giveTitleSort: "   ",
			giveTitle:     "Blade Runner",
			want:          "B",
		},
		{
			name:          "neither set names nothing",
			giveTitleSort: "  ",
			want:          JumpOtherKey,
		},
		{
			name:      "a title starting with a digit is not a letter",
			giveTitle: "1999",
			want:      JumpOtherKey,
		},
		{
			name:      "an accented first letter is outside A-Z",
			giveTitle: "Émile",
			want:      JumpOtherKey,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, TitleJumpKey(test.giveTitleSort, test.giveTitle))
		})
	}
}
