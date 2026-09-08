// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/queue"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

const (
	idOldClip = "old-clip"
	idNewGIF  = "new-gif"
	idTieZ    = "tie-z"
	idTieA    = "tie-a"
)

func TestParseClipListQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give string
		want clipListQuery
	}{
		{
			give: "/clips",
			want: clipListQuery{Sort: clipSortCreatedDesc},
		},
		{
			give: "/clips?type=gif&q=Intro&sort=name_asc&status=completed",
			want: clipListQuery{
				Status: view.ClipStatusCompleted,
				Type:   clipTypeGIF,
				Query:  "Intro",
				Sort:   clipSortNameAsc,
			},
		},
		{
			give: "/clips?type=nope&sort=bogus&q=%20Clip%20",
			want: clipListQuery{Query: "Clip", Sort: clipSortCreatedDesc},
		},
		{
			give: "/clips?type=screenshot&sort=updated_desc",
			want: clipListQuery{Type: clipTypeScreenshot, Sort: clipSortUpdatedDesc},
		},
	}

	for _, test := range tests {
		t.Run(test.give, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, parseClipListQueryFrom(t, test.give))
		})
	}
}

func TestApplyClipListQuery(t *testing.T) {
	t.Parallel()

	older := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	latest := newer.Add(time.Hour)

	jobs := []*queue.Job{
		listJob(
			idOldClip,
			"Alpha",
			testMovie,
			clipTypeClip,
			view.ClipStatusCompleted,
			older,
			latest,
		),
		listJob(idNewGIF, "bravo", "Other", clipTypeGIF, view.ClipStatusPending, newer, older),
		listJob(idTieZ, "", testMovie, clipTypeClip, view.ClipStatusProcessing, newer, newer),
		listJob(idTieA, "", testMovie, clipTypeScreenshot, view.ClipStatusFailed, newer, newer),
	}

	tests := []struct {
		name  string
		query clipListQuery
		want  []string
	}{
		{
			name:  "default newest created then id",
			query: normalizeClipListQuery(clipListQuery{}),
			want:  []string{idTieZ, idTieA, idNewGIF, idOldClip},
		},
		{
			name:  "oldest created",
			query: clipListQuery{Sort: clipSortCreatedAsc},
			want:  []string{idOldClip, idTieZ, idTieA, idNewGIF},
		},
		{
			name:  "recently modified",
			query: clipListQuery{Sort: clipSortUpdatedDesc},
			want:  []string{idOldClip, idTieZ, idTieA, idNewGIF},
		},
		{
			name:  "oldest modified",
			query: clipListQuery{Sort: clipSortUpdatedAsc},
			want:  []string{idNewGIF, idTieZ, idTieA, idOldClip},
		},
		{
			name:  "name ascending uses display name",
			query: clipListQuery{Sort: clipSortNameAsc},
			want:  []string{idOldClip, idNewGIF, idTieZ, idTieA},
		},
		{
			name:  "name descending",
			query: clipListQuery{Sort: clipSortNameDesc},
			want:  []string{idTieZ, idTieA, idNewGIF, idOldClip},
		},
		{
			name:  "type gif",
			query: clipListQuery{Type: clipTypeGIF, Sort: clipSortCreatedDesc},
			want:  []string{idNewGIF},
		},
		{
			name:  "name matches clip name",
			query: clipListQuery{Query: "alpha", Sort: clipSortCreatedDesc},
			want:  []string{idOldClip},
		},
		{
			name:  "name matches media title",
			query: clipListQuery{Query: defaultMediaType, Sort: clipSortCreatedDesc},
			want:  []string{idTieZ, idTieA, idOldClip},
		},
		{
			name:  "pending includes processing",
			query: clipListQuery{Status: view.ClipStatusPending, Sort: clipSortCreatedDesc},
			want:  []string{idTieZ, idNewGIF},
		},
		{
			name: "status and type together",
			query: clipListQuery{
				Status: view.ClipStatusPending,
				Type:   clipTypeGIF,
				Sort:   clipSortCreatedDesc,
			},
			want: []string{idNewGIF},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := applyClipListQuery(jobs, test.query)
			require.Len(t, got, len(test.want))
			assert.Equal(t, test.want, clipJobIDs(got))
		})
	}
}

func TestClipListQueryFiltered(t *testing.T) {
	t.Parallel()

	assert.False(t, clipListQuery{Sort: clipSortCreatedDesc}.filtered())
	assert.True(t, clipListQuery{Type: clipTypeGIF}.filtered())
	assert.True(t, clipListQuery{Query: "x"}.filtered())
	assert.True(t, clipListQuery{Status: view.ClipStatusFailed}.filtered())
}

func parseClipListQueryFrom(t *testing.T, target string) clipListQuery {
	t.Helper()

	app := fiber.New()
	var parsed clipListQuery

	app.Get("/clips", func(ctx fiber.Ctx) error {
		parsed = parseClipListQuery(ctx)

		return nil
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	_, _ = io.Copy(io.Discard, resp.Body)

	return parsed
}

func listJob(
	id, name, mediaTitle, clipType, status string,
	created, updated time.Time,
) *queue.Job {
	return &queue.Job{
		ID:            id,
		Type:          queue.JobType(clipType),
		Name:          name,
		MediaID:       "",
		MediaTitle:    mediaTitle,
		MediaType:     "",
		InputPath:     "",
		OutputPath:    "",
		StartTime:     0,
		Duration:      0,
		Quality:       "",
		Width:         0,
		FPS:           0,
		AudioIndex:    0,
		CropBlackBars: false,
		Status:        queue.JobStatus(status),
		Progress:      0,
		Error:         "",
		CreatedAt:     created,
		UpdatedAt:     updated,
	}
}

func clipJobIDs(jobs []*queue.Job) []string {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}

	return ids
}
