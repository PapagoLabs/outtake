// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/database/mocks"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/queue"
)

// testMovieType is the media type used by database fixtures.
const testMovieType = "movie"

func TestClipPersistence(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := &queue.Job{
		ID:            "clip-1",
		Type:          queue.JobTypeClip,
		Name:          "Intro",
		MediaID:       "100",
		MediaTitle:    "Test Movie",
		MediaType:     testMovieType,
		InputPath:     "/media/movie.mkv",
		OutputPath:    "/out/clip-1.mp4",
		StartTime:     10,
		Duration:      15,
		Quality:       "medium",
		Width:         0,
		FPS:           0,
		AudioIndex:    1,
		CropBlackBars: true,
		Status:        queue.JobStatusPending,
		Progress:      0,
		Error:         "",
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	}

	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.MediaTitle, got.MediaTitle)
	assert.Equal(t, "Intro", got.Name)
	assert.Equal(t, 1, got.AudioIndex)
	assert.True(t, got.CropBlackBars)
	assert.False(t, got.WebSafeColor)

	job.WebSafeColor = true
	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err = db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.True(t, got.WebSafeColor)

	byMedia, err := db.ListClipsForMedia(t.Context(), "100")
	require.NoError(t, err)
	require.Len(t, byMedia, 1)
	assert.Equal(t, queue.JobStatusPending, got.Status)

	pending, err := db.ListPendingClips(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)

	job.Status = queue.JobStatusCompleted
	job.Progress = 100
	require.NoError(t, db.SaveClip(t.Context(), job))

	pending, err = db.ListPendingClips(t.Context())
	require.NoError(t, err)
	assert.Empty(t, pending)

	all, err := db.ListClips(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, queue.JobStatusCompleted, all[0].Status)

	require.NoError(t, db.DeleteClip(t.Context(), job.ID))

	_, err = db.GetClip(t.Context(), job.ID)
	require.ErrorIs(t, err, ErrClipNotFound)
}

func TestListClipsOrder(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips-order.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	older := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)

	require.NoError(t, db.SaveClip(t.Context(), testStoredClip("old", "100", older)))
	require.NoError(t, db.SaveClip(t.Context(), testStoredClip("tie-a", "100", newer)))
	require.NoError(t, db.SaveClip(t.Context(), testStoredClip("tie-z", "100", newer)))
	require.NoError(t, db.SaveClip(t.Context(), testStoredClip("other", "200", newer)))

	wantAll := []string{"tie-z", "tie-a", "other", "old"}
	all, err := db.ListClips(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 4)
	assert.Equal(t, wantAll, clipIDs(all))

	wantMedia := []string{"tie-z", "tie-a", "old"}
	byMedia, err := db.ListClipsForMedia(t.Context(), "100")
	require.NoError(t, err)
	require.Len(t, byMedia, 3)
	assert.Equal(t, wantMedia, clipIDs(byMedia))
}

func testStoredClip(id, mediaID string, created time.Time) *queue.Job {
	return &queue.Job{
		ID:            id,
		Type:          queue.JobTypeClip,
		Name:          id,
		MediaID:       mediaID,
		MediaTitle:    "Order Movie",
		MediaType:     "show",
		InputPath:     "/media/order.mkv",
		OutputPath:    "/out/" + id + ".mp4",
		StartTime:     10,
		Duration:      15,
		Quality:       "high",
		Width:         0,
		FPS:           0,
		AudioIndex:    0,
		CropBlackBars: false,
		Status:        queue.JobStatusCompleted,
		Progress:      100,
		Error:         "",
		CreatedAt:     created,
		UpdatedAt:     created,
	}
}

// TestSaveClipUpdatesGIFDimensionsOnConflict is the regression guard for the
// conflict path of SaveClip.
//
// Width and fps are in the insert list but were missing from the ON CONFLICT
// clause, so editing a GIF's size and saving looked successful while the stored
// row kept the old values. A regenerate then re-encoded at the original size.
func TestSaveClipUpdatesGIFDimensionsOnConflict(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips-gif.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := testStoredClip("gif-1", "100", time.Now().UTC().Truncate(time.Second))

	job.Type = queue.JobTypeGIF
	job.Width = 1280
	job.FPS = 24
	require.NoError(t, db.SaveClip(t.Context(), job))

	// The edit form posts a new size for the same clip.
	job.Width = 720
	job.FPS = 12
	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err := db.GetClip(t.Context(), "gif-1")
	require.NoError(t, err)
	assert.Equal(t, 720, got.Width, "an edited GIF width must survive a resave")
	assert.Equal(t, 12, got.FPS, "an edited GIF frame rate must survive a resave")
}

// TestSaveClipKeepsZeroDimensionsForVideoClips records that writing the
// dimensions on every update is a no-op for video clips, which store zero to
// mean "use the default" and never carry a GIF size.
func TestSaveClipKeepsZeroDimensionsForVideoClips(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips-video.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := testStoredClip("clip-1", "100", time.Now().UTC().Truncate(time.Second))

	job.Width = 0
	job.FPS = 0
	require.NoError(t, db.SaveClip(t.Context(), job))

	// Editing an unrelated field must not disturb the dimensions.
	job.Name = "Renamed"
	job.StartTime = 42
	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err := db.GetClip(t.Context(), "clip-1")
	require.NoError(t, err)
	assert.Equal(t, 0, got.Width)
	assert.Equal(t, 0, got.FPS)
	assert.Equal(t, "Renamed", got.Name)
	assert.InDelta(t, 42, got.StartTime, 0.0005)
}

// TestSaveClipKeepsImmutableColumnsOnConflict pins the columns the conflict
// clause deliberately leaves alone, so a future "just update everything"
// change does not quietly reset a clip's source or creation time.
func TestSaveClipKeepsImmutableColumnsOnConflict(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips-immutable.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	created := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	job := testStoredClip("clip-1", "100", created)

	job.Width = 640
	job.FPS = 15
	require.NoError(t, db.SaveClip(t.Context(), job))

	job.MediaID = "999"
	job.MediaTitle = "Other Title"
	job.MediaType = testMovieType
	job.InputPath = "/media/other.mkv"
	job.CreatedAt = time.Now().UTC().Truncate(time.Second)
	job.Width = 1920
	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err := db.GetClip(t.Context(), "clip-1")
	require.NoError(t, err)
	assert.Equal(t, "100", got.MediaID)
	assert.Equal(t, "Order Movie", got.MediaTitle)
	assert.Equal(t, "show", got.MediaType)
	assert.Equal(t, "/media/order.mkv", got.InputPath)
	assert.WithinDuration(t, created, got.CreatedAt, time.Second)
	assert.Equal(t, 1920, got.Width, "the editable column still updates")
}

func clipIDs(jobs []*queue.Job) []string {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}

	return ids
}

func TestScanJobMockScannable(t *testing.T) {
	t.Parallel()

	row := mocks.NewMockScannable(t)
	row.EXPECT().Scan(mock.Anything).Return(nil)

	job, err := scanJob(row)
	require.NoError(t, err)
	require.NotNil(t, job)
}

func TestTokenAndServerPersistence(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/tokens.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.SaveToken(t.Context(), "client-a", "token-1"))
	require.NoError(t, db.SaveToken(t.Context(), "client-a", "token-2"))

	token, err := db.LatestToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "token-2", token)

	server := plex.Server{
		Name:    "Home",
		Address: "192.168.1.5",
		Port:    32400,
		Scheme:  "http",
		Token:   "srv-token",
		Local:   false,
	}
	require.NoError(t, db.SaveSelectedServer(t.Context(), server))

	got, ok, err := db.SelectedServer(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, server, got)

	require.NoError(t, db.ClearAuth(t.Context()))

	token, err = db.LatestToken(t.Context())
	require.NoError(t, err)
	assert.Empty(t, token)

	_, ok, err = db.SelectedServer(t.Context())
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMigrateIdempotent(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/repeat.db"

	db, err := New(path)
	require.NoError(t, err)
	require.NoError(t, db.SaveToken(t.Context(), "c", "t"))
	require.NoError(t, db.Close())

	db, err = New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	token, err := db.LatestToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "t", token)
}
