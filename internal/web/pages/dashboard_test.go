// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestDashboardCountsClips(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Dashboard(DashboardProps{
		TotalClips:   12,
		PendingClips: 3,
		Completed:    8,
		Failed:       1,
		Sessions:     nil,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	for _, want := range []string{
		`href="/clips"`,
		`href="/clips?status=pending"`,
		`href="/clips?status=completed"`,
		`href="/clips?status=failed"`,
		">Total Clips<",
		">Pending<",
		">Completed<",
		">Failed<",
		">12<",
		">3<",
		">8<",
		">1<",
		"text-yellow-500",
		"text-green-500",
		"text-destructive",
	} {
		assert.Contains(t, body, want)
	}

	assert.Contains(t, body, "No one is playing anything in Plex right now.")
	assert.Contains(t, body, "Browse Media")
	assert.Contains(t, body, "View Clips")
	assert.Contains(t, body, `data-nav="dashboard"`)
}

func TestLiveSessionsEmptyState(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := LiveSessions(nil, false).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `id="live-sessions"`)
	assert.Contains(t, body, "No one is playing anything in Plex right now.")
	assert.NotContains(t, body, "Clip now")
}

func TestLiveSessionsOmitsClipNowWithoutAMediaItem(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := LiveSessions([]view.SessionItem{{
		ID:    "sess-3",
		Parts: []view.Crumb{{Title: "Movie"}},
		Year:  1995,
	}}, false).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "Movie", "a crumb with no link stays plain text")
	assert.Contains(t, body, "(1995)")
	assert.NotContains(t, body, "Clip now",
		"without a media item there is nothing to clip from")
	assert.NotContains(t, body, "<a", "a crumb with no URL is not linked")
}

func TestLiveSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		give        []view.SessionItem
		contains    []string
		notContains []string
	}{
		{
			name: "episode links show season and episode",
			give: []view.SessionItem{{
				ID:      "sess-1",
				MediaID: "42",
				Parts: []view.Crumb{
					{Title: "Show", URL: "/media?library=2&parent=9&title=Show"},
					{
						Title: "Season 3",
						URL:   "/media?library=2&parent=10&title=Season+3&up=9&upTitle=Show",
					},
					{Title: "Episode 5", URL: "/media/item/42"},
				},
				ViewOffset: 10345 * time.Millisecond,
				Duration:   120 * time.Second,
			}},
			contains: []string{
				`href="/media?library=2&amp;parent=9&amp;title=Show"`,
				`href="/media?library=2&amp;parent=10&amp;title=Season+3&amp;up=9&amp;upTitle=Show"`,
				`href="/media/item/42"`,
				"Show",
				"Season 3",
				"Episode 5",
				" · ",
				`href="/media/item/42?start=10.345"`,
				"Clip now",
			},
		},
		{
			name: "movie title links and year stays plain",
			give: []view.SessionItem{{
				ID:         "sess-2",
				MediaID:    "100",
				Parts:      []view.Crumb{{Title: "Movie", URL: "/media/item/100"}},
				Year:       1995,
				ViewOffset: 30007 * time.Millisecond,
				Duration:   600 * time.Second,
			}},
			contains: []string{
				`href="/media/item/100"`,
				"Movie",
				"(1995)",
				`href="/media/item/100?start=30.007"`,
			},
			notContains: []string{
				"Movie" + " · (1995)",
				`href="/media/item/100">` + "Movie" + " (1995)",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			err := LiveSessions(test.give, false).Render(t.Context(), &buf)
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
