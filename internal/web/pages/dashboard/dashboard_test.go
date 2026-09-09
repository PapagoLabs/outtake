// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package dashboard

import (
	"strings"
	"testing"

	viewmedia "github.com/PapagoLabs/outtake/internal/web/view/media"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMovie = "Movie"

func TestLiveSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		give        []SessionItem
		contains    []string
		notContains []string
	}{
		{
			name: "episode links show season and episode",
			give: []SessionItem{{
				ID:      "sess-1",
				MediaID: "42",
				Parts: []viewmedia.Crumb{
					{Title: "Show", URL: "/media?library=2&parent=9&title=Show"},
					{
						Title: "Season 3",
						URL:   "/media?library=2&parent=10&title=Season+3&up=9&upTitle=Show",
					},
					{Title: "Episode 5", URL: "/media/item/42"},
				},
				ViewOffset: 10,
				Duration:   120,
			}},
			contains: []string{
				`href="/media?library=2&amp;parent=9&amp;title=Show"`,
				`href="/media?library=2&amp;parent=10&amp;title=Season+3&amp;up=9&amp;upTitle=Show"`,
				`href="/media/item/42"`,
				"Show",
				"Season 3",
				"Episode 5",
				" · ",
				`href="/media/item/42?start=10.0"`,
				"Clip now",
			},
		},
		{
			name: "movie title links and year stays plain",
			give: []SessionItem{{
				ID:         "sess-2",
				MediaID:    "100",
				Parts:      []viewmedia.Crumb{{Title: testMovie, URL: "/media/item/100"}},
				Year:       1995,
				ViewOffset: 30,
				Duration:   600,
			}},
			contains: []string{
				`href="/media/item/100"`,
				testMovie,
				"(1995)",
				`href="/media/item/100?start=30.0"`,
			},
			notContains: []string{
				testMovie + " · (1995)",
				`href="/media/item/100">` + testMovie + " (1995)",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			err := LiveSessions(test.give).Render(t.Context(), &buf)
			require.NoError(t, err)

			body := buf.String()
			for _, want := range test.contains {
				assert.Contains(t, body, want)
			}

			for _, notWant := range test.notContains {
				assert.NotContains(t, body, notWant)
			}
		})
	}
}
