// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testMediaTitle = "Movie"

func TestClipItemDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give ClipItem
		want string
	}{
		{
			name: "named clip",
			give: ClipItem{Name: "Intro", MediaTitle: testMediaTitle},
			want: "Intro",
		},
		{
			name: "falls back to media title",
			give: ClipItem{Name: "", MediaTitle: testMediaTitle},
			want: testMediaTitle,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.give.DisplayName())
		})
	}
}

func TestClipItemIsActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want bool
	}{
		{name: ClipStatusPending, give: ClipStatusPending, want: true},
		{name: ClipStatusProcessing, give: ClipStatusProcessing, want: true},
		{name: ClipStatusCompleted, give: ClipStatusCompleted, want: false},
		{name: "failed", give: "failed", want: false},
		{name: "empty", give: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, ClipItem{Status: test.give}.IsActive())
		})
	}
}

func TestClipItemCanPlay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give ClipItem
		want bool
	}{
		{
			name: "completed with file",
			give: ClipItem{Status: ClipStatusCompleted, FileExists: true},
			want: true,
		},
		{
			name: "completed missing file",
			give: ClipItem{Status: ClipStatusCompleted, FileExists: false},
			want: false,
		},
		{
			name: "processing with file",
			give: ClipItem{Status: ClipStatusProcessing, FileExists: true},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.give.CanPlay())
		})
	}
}
