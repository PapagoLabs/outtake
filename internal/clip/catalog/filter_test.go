// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
)

func listJob(
	id, name, mediaTitle string,
	clipType clip.Type,
	status clip.Status,
	created, updated time.Time,
) *clip.Job {
	return &clip.Job{
		ID:         id,
		Type:       clipType,
		Name:       name,
		MediaTitle: mediaTitle,

		CreatedAt: created,
		UpdatedAt: updated, Status: status,
	}
}

func clipJobIDs(jobs []*clip.Job) []string {
	ids := make([]string, 0, len(jobs))

	for _, job := range jobs {
		ids = append(ids, job.ID)
	}

	return ids
}

// clipFixtures returns four clips that between them separate every sort and
// filter rule, with the ids the expectations are written against.
func clipFixtures() []*clip.Job {
	older := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	latest := newer.Add(time.Hour)

	return []*clip.Job{
		listJob(
			"old-clip",
			"Alpha",
			"Movie",
			clip.TypeClip,
			clip.StatusCompleted,
			older,
			latest,
		),
		listJob("new-gif", "bravo", "Other", clip.TypeGIF, clip.StatusPending, newer, older),
		listJob("tie-z", "", "Movie", clip.TypeClip, clip.StatusProcessing, newer, newer),
		listJob("tie-a", "", "Movie", clip.TypeScreenshot, clip.StatusFailed, newer, newer),
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ClipListQuery{Sort: SortCreatedDesc}, Normalize(ClipListQuery{}),
		"a query that names nothing lists newest first")

	assert.Equal(t,
		ClipListQuery{Query: "  Clip  ", Sort: SortCreatedDesc},
		Normalize(ClipListQuery{Query: "  Clip  ", Sort: "bogus"}),
		"a sort this app does not know falls back to the default")

	assert.Equal(t,
		ClipListQuery{Sort: SortCreatedDesc},
		Normalize(ClipListQuery{Type: "hologram", Sort: SortCreatedDesc}),
		"a type this app does not produce is dropped, so it matches nothing")

	assert.Equal(t,
		ClipListQuery{Status: "failed", Type: "gif", Sort: SortNameAsc},
		Normalize(ClipListQuery{Status: "failed", Type: "gif", Sort: SortNameAsc}),
		"a query that is already normal is returned unchanged")
}

func TestClipListQueryFiltered(t *testing.T) {
	t.Parallel()

	assert.False(t, ClipListQuery{Sort: SortCreatedDesc}.Filtered(),
		"a sort alone is not a filter")
	assert.True(t, ClipListQuery{Type: "gif"}.Filtered())
	assert.True(t, ClipListQuery{Query: "x"}.Filtered())
	assert.True(t, ClipListQuery{Status: "failed"}.Filtered())
}

func TestApply(t *testing.T) {
	t.Parallel()

	jobs := clipFixtures()

	tests := []struct {
		name  string
		query ClipListQuery
		want  []string
	}{
		{
			name:  "default newest created then id",
			query: Normalize(ClipListQuery{}),
			want:  []string{"tie-z", "tie-a", "new-gif", "old-clip"},
		},
		{
			name:  "oldest created",
			query: ClipListQuery{Sort: SortCreatedAsc},
			want:  []string{"old-clip", "tie-z", "tie-a", "new-gif"},
		},
		{
			name:  "recently modified",
			query: ClipListQuery{Sort: SortUpdatedDesc},
			want:  []string{"old-clip", "tie-z", "tie-a", "new-gif"},
		},
		{
			name:  "oldest modified",
			query: ClipListQuery{Sort: SortUpdatedAsc},
			want:  []string{"new-gif", "tie-z", "tie-a", "old-clip"},
		},
		{
			name:  "name ascending uses the display name",
			query: ClipListQuery{Sort: SortNameAsc},
			want:  []string{"old-clip", "new-gif", "tie-z", "tie-a"},
		},
		{
			name:  "name descending",
			query: ClipListQuery{Sort: SortNameDesc},
			want:  []string{"tie-z", "tie-a", "new-gif", "old-clip"},
		},
		{
			name:  "type gif",
			query: ClipListQuery{Type: "gif", Sort: SortCreatedDesc},
			want:  []string{"new-gif"},
		},
		{
			name:  "name matches a clip name",
			query: ClipListQuery{Query: "alpha", Sort: SortCreatedDesc},
			want:  []string{"old-clip"},
		},
		{
			name:  "name matches a media title",
			query: ClipListQuery{Query: clip.DefaultMediaType, Sort: SortCreatedDesc},
			want:  []string{"tie-z", "tie-a", "old-clip"},
		},
		{
			name:  "pending includes processing",
			query: ClipListQuery{Status: "pending", Sort: SortCreatedDesc},
			want:  []string{"tie-z", "new-gif"},
		},
		{
			name: "status and type together",
			query: ClipListQuery{
				Status: "pending",
				Type:   "gif",
				Sort:   SortCreatedDesc,
			},
			want: []string{"new-gif"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := Apply(jobs, test.query)
			require.Len(t, got, len(test.want))
			assert.Equal(t, test.want, clipJobIDs(got))
		})
	}
}

func TestApplyBreaksATieOnID(t *testing.T) {
	t.Parallel()

	// Two clips can share a display name, and the same created time down to the
	// millisecond. They must still come back in one order every time, or a row
	// would swap places between two polls of the same list.
	stamp := time.Unix(0, 0).UTC()

	jobs := []*clip.Job{
		listJob("a", "", "Movie", clip.TypeClip, clip.StatusCompleted, stamp, stamp),
		listJob("b", "", "Movie", clip.TypeClip, clip.StatusCompleted, stamp, stamp),
	}

	want := clipJobIDs(Apply(jobs, ClipListQuery{Sort: SortNameAsc}))

	jobs[0], jobs[1] = jobs[1], jobs[0]

	assert.Equal(t,
		want,
		clipJobIDs(Apply(jobs, ClipListQuery{Sort: SortNameAsc})),
		"the order must not depend on the order the clips arrived in")
}
