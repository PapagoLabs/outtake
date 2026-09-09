// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	viewclip "github.com/PapagoLabs/outtake/internal/web/view/clip"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared"
)

const testMovie = "Movie"

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
		want ListQuery
	}{
		{
			give: "/clips",
			want: ListQuery{Sort: clipSortCreatedDesc},
		},
		{
			give: "/clips?type=gif&q=Intro&sort=name_asc&status=completed",
			want: ListQuery{
				Status: viewclip.ClipStatusCompleted,
				Type:   clipTypeGIF,
				Query:  "Intro",
				Sort:   clipSortNameAsc,
			},
		},
		{
			give: "/clips?type=nope&sort=bogus&q=%20Clip%20",
			want: ListQuery{Query: "Clip", Sort: clipSortCreatedDesc},
		},
		{
			give: "/clips?type=screenshot&sort=updated_desc",
			want: ListQuery{Type: clipTypeScreenshot, Sort: clipSortUpdatedDesc},
		},
	}

	for _, test := range tests {
		t.Run(test.give, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, ParseListQueryFrom(t, test.give))
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
			viewclip.ClipStatusCompleted,
			older,
			latest,
		),
		listJob(idNewGIF, "bravo", "Other", clipTypeGIF, viewclip.ClipStatusPending, newer, older),
		listJob(idTieZ, "", testMovie, clipTypeClip, viewclip.ClipStatusProcessing, newer, newer),
		listJob(idTieA, "", testMovie, clipTypeScreenshot, viewclip.ClipStatusFailed, newer, newer),
	}

	tests := []struct {
		name  string
		query ListQuery
		want  []string
	}{
		{
			name:  "default newest created then id",
			query: NormalizeListQuery(ListQuery{}),
			want:  []string{idTieZ, idTieA, idNewGIF, idOldClip},
		},
		{
			name:  "oldest created",
			query: ListQuery{Sort: clipSortCreatedAsc},
			want:  []string{idOldClip, idTieZ, idTieA, idNewGIF},
		},
		{
			name:  "recently modified",
			query: ListQuery{Sort: clipSortUpdatedDesc},
			want:  []string{idOldClip, idTieZ, idTieA, idNewGIF},
		},
		{
			name:  "oldest modified",
			query: ListQuery{Sort: clipSortUpdatedAsc},
			want:  []string{idNewGIF, idTieZ, idTieA, idOldClip},
		},
		{
			name:  "name ascending uses display name",
			query: ListQuery{Sort: clipSortNameAsc},
			want:  []string{idOldClip, idNewGIF, idTieZ, idTieA},
		},
		{
			name:  "name descending",
			query: ListQuery{Sort: clipSortNameDesc},
			want:  []string{idTieZ, idTieA, idNewGIF, idOldClip},
		},
		{
			name:  "type gif",
			query: ListQuery{Type: clipTypeGIF, Sort: clipSortCreatedDesc},
			want:  []string{idNewGIF},
		},
		{
			name:  "name matches clip name",
			query: ListQuery{Query: "alpha", Sort: clipSortCreatedDesc},
			want:  []string{idOldClip},
		},
		{
			name:  "name matches media title",
			query: ListQuery{Query: shared.DefaultMediaType, Sort: clipSortCreatedDesc},
			want:  []string{idTieZ, idTieA, idOldClip},
		},
		{
			name:  "pending includes processing",
			query: ListQuery{Status: viewclip.ClipStatusPending, Sort: clipSortCreatedDesc},
			want:  []string{idTieZ, idNewGIF},
		},
		{
			name: "status and type together",
			query: ListQuery{
				Status: viewclip.ClipStatusPending,
				Type:   clipTypeGIF,
				Sort:   clipSortCreatedDesc,
			},
			want: []string{idNewGIF},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := ApplyListQuery(jobs, test.query)
			require.Len(t, got, len(test.want))
			assert.Equal(t, test.want, clipJobIDs(got))
		})
	}
}

func TestClipListQueryFiltered(t *testing.T) {
	t.Parallel()

	assert.False(t, ListQuery{Sort: clipSortCreatedDesc}.Filtered())
	assert.True(t, ListQuery{Type: clipTypeGIF}.Filtered())
	assert.True(t, ListQuery{Query: "x"}.Filtered())
	assert.True(t, ListQuery{Status: viewclip.ClipStatusFailed}.Filtered())
}

func ParseListQueryFrom(t *testing.T, target string) ListQuery {
	t.Helper()

	app := fiber.New()
	var parsed ListQuery

	app.Get("/clips", func(ctx fiber.Ctx) error {
		parsed = ParseListQuery(ctx)

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
