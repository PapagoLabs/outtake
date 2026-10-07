// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// catalogFixture describes one clip in the shared fixture set.
type catalogFixture struct {
	id         string
	name       string
	mediaID    string
	mediaTitle string
	kind       clip.Type
	status     clip.Status
	created    time.Duration
	updated    time.Duration
}

// catalogBase is the instant every fixture clip is stamped from.
var catalogBase = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

// fixtures is a realistic catalog: two sources, three clip types, every status,
// a clip that borrows its source's title, and creation and update times that tie.
func fixtures() []catalogFixture {
	return []catalogFixture{
		{
			id: "b-intro", name: "Intro", mediaID: "100", mediaTitle: "Feature Movie",
			kind: clip.TypeClip, status: clip.StatusCompleted,
			created: 0, updated: 3 * time.Minute,
		},
		{
			id:         "a-intro",
			name:       "intro reprise",
			mediaID:    "100",
			mediaTitle: "Feature Movie",
			kind:       clip.TypeClip,
			status:     clip.StatusPending,
			created:    0,
			updated:    time.Minute,
		},
		{
			id: "c-outro", name: "Outro", mediaID: "100", mediaTitle: "Feature Movie",
			kind: clip.TypeGIF, status: clip.StatusProcessing,
			created: 5 * time.Minute, updated: 5 * time.Minute,
		},
		{
			id: "d-gif", name: "Animated Intro", mediaID: "100", mediaTitle: "Feature Movie",
			kind: clip.TypeGIF, status: clip.StatusPending,
			created: 5 * time.Minute, updated: 2 * time.Minute,
		},
		{
			id: "e-shot", name: "", mediaID: "100", mediaTitle: "Feature Movie",
			kind: clip.TypeScreenshot, status: clip.StatusFailed,
			created: 2 * time.Minute, updated: 2 * time.Minute,
		},
		{
			id: "f-teaser", name: "Teaser", mediaID: "200", mediaTitle: "Show Episode",
			kind: clip.TypeClip, status: clip.StatusCancelled,
			created: time.Minute, updated: 6 * time.Minute,
		},
		{
			id: "g-opening", name: "Opening", mediaID: "200", mediaTitle: "Show Episode",
			kind: clip.TypeGIF, status: clip.StatusCompleted,
			created: 9 * time.Minute, updated: 9 * time.Minute,
		},
		{
			id: "h-credits", name: "Credits", mediaID: "200", mediaTitle: "Show Episode",
			kind: clip.TypeScreenshot, status: clip.StatusCompleted,
			created: 4 * time.Minute, updated: 4 * time.Minute,
		},
	}
}

// fixtureClips turns the fixture set into clips stamped from the shared base.
//
// Returns:
//   - clips: The fixture clips, in fixture order.
func fixtureClips() []*clip.Job {
	described := fixtures()
	clips := make([]*clip.Job, 0, len(described))

	for _, item := range described {
		clips = append(clips, &clip.Job{
			ID:         item.id,
			Type:       item.kind,
			Name:       item.name,
			MediaID:    item.mediaID,
			MediaTitle: item.mediaTitle,
			MediaType:  clip.DefaultMediaType,

			StartTime: 2 * time.Second,
			Duration:  6 * time.Second,
			Quality:   "medium",

			CropBlackBars: item.id == "d-gif",
			WebSafeColor:  item.id == "c-outro",
			PreserveHDR:   item.id == "g-opening",
			CreatedAt:     catalogBase.Add(item.created),
			UpdatedAt: catalogBase.Add(
				item.updated,
			),
			InputPath:  "/media/" + item.mediaTitle + ".mkv",
			OutputPath: "/output/" + item.id + ".mp4",

			Status:   item.status,
			Progress: progressFor(item.status),
		})
	}

	return clips
}

// progressFor returns the progress a clip settles at for its status.
//
// Parameters:
//   - status: Status the clip reached.
//
// Returns:
//   - percent: Percent complete, zero for a clip that never ran.
func progressFor(status clip.Status) int {
	if status == clip.StatusCompleted || status == clip.StatusProcessing {
		return 100
	}

	return 0
}

// clipIDs returns the identifiers of clips in the order they were given.
//
// Parameters:
//   - clips: Clips to read.
//
// Returns:
//   - ids: The identifiers, in order.
func clipIDs(clips []*clip.Job) []string {
	ids := make([]string, 0, len(clips))
	for _, item := range clips {
		ids = append(ids, item.ID)
	}

	return ids
}

// catalogDatabase returns a migrated SQLite database holding the fixture set.
//
// Parameters:
//   - t: The test that needs the database.
//
// Returns:
//   - db: A migrated database that is closed when the test finishes.
func catalogDatabase(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.New(filepath.Join(t.TempDir(), "catalog.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	for _, item := range fixtureClips() {
		require.NoError(t, db.SaveClip(t.Context(), item))
	}

	return db
}

func TestIntegration_ApplyCombinesStatusTypeAndNameFilters(t *testing.T) {
	t.Parallel()

	clips := fixtureClips()

	every := catalog.Apply(clips, catalog.Normalize(catalog.ClipListQuery{}))
	assert.Len(t, every, len(clips), "no filters lists everything")

	pendingOnly := catalog.Apply(clips, catalog.Normalize(catalog.ClipListQuery{
		Status: clip.StatusPending,
	}))
	assert.Equal(t, []string{"d-gif", "c-outro", "a-intro"}, clipIDs(pendingOnly),
		"pending takes processing with it and the rest fall to the id tiebreak")

	gifsOnly := catalog.Apply(clips, catalog.Normalize(catalog.ClipListQuery{
		Type: clip.TypeGIF,
	}))
	assert.Equal(t, []string{"g-opening", "d-gif", "c-outro"}, clipIDs(gifsOnly))

	named := catalog.Apply(clips, catalog.Normalize(catalog.ClipListQuery{
		Query: "INTRO",
	}))
	assert.Equal(t, []string{"d-gif", "b-intro", "a-intro"}, clipIDs(named),
		"the name search is case-insensitive and reaches the clip names")

	bySourceTitle := catalog.Apply(clips, catalog.Normalize(catalog.ClipListQuery{
		Query: "Show Episode",
	}))
	assert.Equal(t, []string{"g-opening", "h-credits", "f-teaser"}, clipIDs(bySourceTitle),
		"a clip with no name of its own is found by its source title")

	combined := catalog.Apply(clips, catalog.Normalize(catalog.ClipListQuery{
		Status: clip.StatusCompleted,
		Type:   clip.TypeGIF,
		Query:  "opening",
	}))
	assert.Equal(t, []string{"g-opening"}, clipIDs(combined))

	impossible := catalog.Apply(clips, catalog.Normalize(catalog.ClipListQuery{
		Status: clip.StatusPending,
		Type:   clip.TypeGIF,
		Query:  "credits",
	}))
	assert.Empty(t, impossible, "filters that cannot both hold match nothing")

	query := catalog.ClipListQuery{Status: clip.StatusPending, Type: clip.TypeGIF}
	assert.True(t, query.Filtered())
	assert.False(t, catalog.ClipListQuery{Sort: catalog.SortNameAsc}.Filtered(),
		"a sort on its own is not a filter")
}

func TestIntegration_NormalizeDropsUnknownFilterAndSortKeys(t *testing.T) {
	t.Parallel()

	normalized := catalog.Normalize(catalog.ClipListQuery{
		Type: "video",
		Sort: "duration_desc",
	})
	assert.Empty(t, normalized.Type)
	assert.Equal(t, catalog.SortCreatedDesc, normalized.Sort)

	kept := catalog.Normalize(catalog.ClipListQuery{
		Status: clip.StatusFailed,
		Type:   clip.TypeScreenshot,
		Sort:   catalog.SortNameDesc,
	})
	assert.Equal(t, clip.StatusFailed, kept.Status)
	assert.Equal(t, clip.TypeScreenshot, kept.Type)
	assert.Equal(t, catalog.SortNameDesc, kept.Sort)
}

func TestIntegration_ApplyOrdersEverySortIncludingTies(t *testing.T) {
	t.Parallel()

	clips := fixtureClips()

	tests := []struct {
		sort     string
		expected []string
	}{
		{
			sort: catalog.SortCreatedDesc,
			expected: []string{
				"g-opening", "d-gif", "c-outro", "h-credits",
				"e-shot", "f-teaser", "b-intro", "a-intro",
			},
		},
		{
			sort: catalog.SortCreatedAsc,
			expected: []string{
				"b-intro", "a-intro", "f-teaser", "e-shot",
				"h-credits", "d-gif", "c-outro", "g-opening",
			},
		},
		{
			sort: catalog.SortUpdatedDesc,
			expected: []string{
				"g-opening", "f-teaser", "c-outro", "h-credits",
				"b-intro", "e-shot", "d-gif", "a-intro",
			},
		},
		{
			sort: catalog.SortUpdatedAsc,
			expected: []string{
				"a-intro", "e-shot", "d-gif", "b-intro",
				"h-credits", "c-outro", "f-teaser", "g-opening",
			},
		},
		{
			sort: catalog.SortNameAsc,
			// The unnamed screenshot sorts under its source title.
			expected: []string{
				"d-gif", "h-credits", "e-shot", "b-intro",
				"a-intro", "g-opening", "c-outro", "f-teaser",
			},
		},
		{
			sort: catalog.SortNameDesc,
			expected: []string{
				"f-teaser", "c-outro", "g-opening", "a-intro",
				"b-intro", "e-shot", "h-credits", "d-gif",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.sort, func(t *testing.T) {
			t.Parallel()

			got := catalog.Apply(clips, catalog.Normalize(catalog.ClipListQuery{Sort: tt.sort}))
			assert.Equal(t, tt.expected, clipIDs(got))
		})
	}
}

func TestIntegration_SummarizeCountsEachStatusOnce(t *testing.T) {
	t.Parallel()

	summary := catalog.Summarize(fixtureClips())

	assert.Equal(t, 8, summary.Total)
	assert.Equal(t, 3, summary.Pending, "pending and processing are one bucket")
	assert.Equal(t, 3, summary.Completed)
	assert.Equal(t, 1, summary.Failed)
	assert.Equal(
		t,
		1,
		summary.Total-summary.Pending-summary.Completed-summary.Failed,
		"the canceled clip is counted in the total but in no bucket",
	)

	empty := catalog.Summarize(nil)
	assert.Equal(t, catalog.Summary{}, empty)
}

func TestIntegration_PagingAFilteredSetCoversEveryClipOnce(t *testing.T) {
	t.Parallel()

	clips := fixtureClips()
	filtered := catalog.Apply(clips, catalog.Normalize(catalog.ClipListQuery{
		Status: clip.StatusCompleted,
	}))
	require.Len(t, filtered, 3)

	summary := catalog.Summarize(filtered)
	assert.Equal(t, len(clips), catalog.Summarize(clips).Total,
		"the unfiltered total is what the toolbar reports")
	assert.Equal(t, 3, summary.Total)
	assert.Equal(t, 3, summary.Completed)
	assert.Zero(t, summary.Pending)
	assert.Zero(t, summary.Failed)

	paged := make([]string, 0, len(filtered))

	for start := 0; start < len(filtered); start += 3 {
		end := min(start+3, len(filtered))

		paged = append(paged, clipIDs(filtered[start:end])...)
	}

	assert.Equal(t, clipIDs(filtered), paged, "paging loses and duplicates nothing")
	assert.Len(t, paged, summary.Total)

	lastPage := catalog.Normalize(catalog.ClipListQuery{})
	assert.Equal(t, catalog.SortCreatedDesc, lastPage.Sort, "an unfiltered listing is newest first")
}

func TestIntegration_ClipResponsesWithholdPathsAndKeepFlags(t *testing.T) {
	t.Parallel()

	clips := catalog.Apply(fixtureClips(), catalog.Normalize(catalog.ClipListQuery{
		Type: clip.TypeGIF,
	}))

	responses := catalog.ClipResponses(clips)
	require.Len(t, responses, 3)

	for _, response := range responses {
		assert.Empty(t, response.InputPath, "the input path is never published")
		assert.Empty(t, response.OutputPath, "the output path is never published")
	}

	assert.Equal(t, "g-opening", responses[0].ID)
	assert.Equal(t, clip.TypeGIF, responses[0].ClipType)
	assert.Equal(t, clip.StatusCompleted, responses[0].Status)
	assert.Equal(t, 100, responses[0].Progress)
	assert.True(t, responses[0].PreserveHDR)

	assert.Equal(t, "d-gif", responses[1].ID)
	assert.Equal(t, clip.StatusPending, responses[1].Status)
	assert.True(t, responses[1].CropBlackBars)

	assert.Equal(t, "c-outro", responses[2].ID)
	assert.Equal(t, clip.StatusProcessing, responses[2].Status)
	assert.True(t, responses[2].WebSafeColor)

	assert.Empty(t, catalog.ClipResponses(nil))
}

func TestIntegration_JobsPrefersTheQueueOverTheDatabase(t *testing.T) {
	t.Parallel()

	db := catalogDatabase(t)
	work := queue.NewQueue(1, func(context.Context, *clip.Job) error { return nil })

	fromDB := catalog.Jobs(t.Context(), work, db)
	assert.Len(t, fromDB, len(fixtureClips()), "an empty queue falls back to storage")

	live := queueJobFixture("live-job")
	work.Restore(live)

	fromQueue := catalog.Jobs(t.Context(), work, db)
	require.Len(t, fromQueue, 1)
	assert.Equal(t, "live-job", fromQueue[0].ID,
		"a queued clip is the whole listing while one is queued")

	found := catalog.Job(t.Context(), work, db, "b-intro")
	require.NotNil(t, found)
	assert.Equal(t, "b-intro", found.ID, "a stored clip is found when the queue lacks it")

	assert.Equal(t, "live-job", catalog.Job(t.Context(), work, db, "live-job").ID)
	assert.Nil(t, catalog.Job(t.Context(), work, db, "absent"))
}

func TestIntegration_ForMediaReturnsOnlyThatSourcesClips(t *testing.T) {
	t.Parallel()

	db := catalogDatabase(t)

	feature := catalog.ForMedia(t.Context(), db, "100")
	assert.Equal(
		t,
		[]string{"d-gif", "c-outro", "e-shot", "b-intro", "a-intro"},
		clipIDs(feature),
		"one source's clips come back newest first",
	)

	trailer := catalog.ForMedia(t.Context(), db, "200")
	assert.Len(t, trailer, 3)

	assert.Empty(t, catalog.ForMedia(t.Context(), db, "absent"))
}

// queueJobFixture returns a queued clip for the catalog source tests.
//
// Parameters:
//   - id: Clip identifier.
//
// Returns:
//   - job: A completed clip the queue tracks without running it.
func queueJobFixture(id string) *clip.Job {
	return &clip.Job{
		ID:         id,
		Type:       clip.TypeClip,
		Name:       "Live",
		MediaID:    "200",
		MediaTitle: "Show Episode",
		MediaType:  clip.DefaultMediaType,
		Quality:    "medium",

		CreatedAt: catalogBase.Add(time.Hour),
		UpdatedAt: catalogBase.Add(time.Hour), Status: clip.StatusCompleted,
		Progress: 100,
	}
}

// renamed returns an edit of a stored clip that changes only its name.
//
// Parameters:
//   - job: The stored clip.
//   - name: The new name.
//
// Returns:
//   - edit: The change.
func renamed(job *clip.Job, name string) clip.Edit {
	return clip.Edit{
		Type:          job.Type,
		Name:          name,
		Quality:       job.Quality,
		Start:         job.StartTime,
		Length:        job.Duration,
		Width:         job.Width,
		FPS:           job.FPS,
		AudioIndex:    job.AudioIndex,
		CropBlackBars: job.CropBlackBars,
		WebSafeColor:  &job.WebSafeColor,
		PreserveHDR:   &job.PreserveHDR,
	}
}

// TestIntegration_UpdateChangesTheQueueAndTheDatabaseTogether covers the
// stale reads an edit used to leave: both stores carry the edit at once.
func TestIntegration_UpdateChangesTheQueueAndTheDatabaseTogether(t *testing.T) {
	t.Parallel()

	db := catalogDatabase(t)
	work := queue.NewQueue(1, nil)

	stored := queueJobFixture("live")
	require.NoError(t, db.SaveClip(t.Context(), stored))
	work.Restore(stored)

	edited, err := catalog.Update(
		t.Context(), work, db, blob.NewPaths(t.TempDir()), "live", renamed(stored, "Renamed"),
	)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", edited.Name)

	assert.Equal(t, "Renamed", catalog.Job(t.Context(), work, db, "live").Name, "a lookup sees it")

	row, err := db.GetClip(t.Context(), "live")
	require.NoError(t, err)
	assert.Equal(t, "Renamed", row.Name, "and so does the next start")
	assert.Equal(t, clip.StatusCompleted, row.Status, "the status is left alone")
}

// TestIntegration_UpdateAdoptsAClipOnlyTheDatabaseHas covers a clip the queue
// was never handed.
func TestIntegration_UpdateAdoptsAClipOnlyTheDatabaseHas(t *testing.T) {
	t.Parallel()

	db := catalogDatabase(t)
	work := queue.NewQueue(1, nil)

	stored, err := db.GetClip(t.Context(), "b-intro")
	require.NoError(t, err)

	_, err = catalog.Update(
		t.Context(), work, db, blob.NewPaths(t.TempDir()), "b-intro", renamed(stored, "Renamed"),
	)
	require.NoError(t, err)

	assert.Equal(t, "Renamed", work.GetJob("b-intro").Name)

	_, err = catalog.Update(
		t.Context(), work, db, blob.NewPaths(t.TempDir()), "absent", renamed(stored, "Renamed"),
	)
	require.ErrorIs(t, err, catalog.ErrClipNotFound)
}

// TestIntegration_ATypeChangeIsRenderedAgain covers a clip whose file no
// longer matches its type: it is queued to render the new type's file.
func TestIntegration_ATypeChangeIsRenderedAgain(t *testing.T) {
	t.Parallel()

	db := catalogDatabase(t)
	work := queue.NewQueue(1, nil)
	paths := blob.NewPaths(t.TempDir())

	// The composition root persists every status the queue reports.
	work.SetStatusFunc(func(job *clip.Job) {
		assert.NoError(t, db.SaveClip(context.WithoutCancel(t.Context()), job))
	})

	stored := queueJobFixture("retyped")

	stored.OutputPath = paths.OutputPath("retyped", clip.TypeClip)
	require.NoError(t, db.SaveClip(t.Context(), stored))
	work.Restore(stored)

	edit := renamed(stored, stored.Name)

	edit.Type = clip.TypeGIF

	edited, err := catalog.Update(t.Context(), work, db, paths, "retyped", edit)
	require.NoError(t, err)

	assert.Equal(t, clip.StatusPending, edited.Status, "the clip is queued again")
	assert.Equal(t, paths.OutputPath("retyped", clip.TypeGIF), edited.OutputPath)

	row, err := db.GetClip(t.Context(), "retyped")
	require.NoError(t, err)
	assert.Equal(t, clip.TypeGIF, row.Type)
	assert.Equal(t, clip.StatusPending, row.Status, "so the next start resumes it")
}

// TestIntegration_UpdateAndRegenerateQueuesTheClip covers the regenerate flag.
func TestIntegration_UpdateAndRegenerateQueuesTheClip(t *testing.T) {
	t.Parallel()

	db := catalogDatabase(t)
	work := queue.NewQueue(1, nil)
	paths := blob.NewPaths(t.TempDir())

	stored := queueJobFixture("again")
	require.NoError(t, db.SaveClip(t.Context(), stored))
	work.Restore(stored)

	edited, err := catalog.UpdateAndRegenerate(
		t.Context(), work, db, paths, "again", renamed(stored, stored.Name),
	)
	require.NoError(t, err)

	assert.Equal(t, clip.StatusPending, edited.Status)
	assert.Equal(t, paths.OutputPath("again", clip.TypeClip), edited.OutputPath)

	_, err = catalog.UpdateAndRegenerate(
		t.Context(), work, db, paths, "again", renamed(stored, stored.Name),
	)
	require.NoError(t, err, "a clip still waiting renders the edit when its turn comes")
}
