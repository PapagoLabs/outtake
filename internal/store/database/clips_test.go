// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

func TestClipPersistence(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := &clip.Job{
		ID:         "clip-1",
		Type:       clip.TypeClip,
		Name:       "Intro",
		MediaID:    "100",
		MediaTitle: "Test Movie",
		MediaType:  "movie",

		StartTime:     10 * time.Second,
		Duration:      15 * time.Second,
		Quality:       "medium",
		Width:         0,
		FPS:           0,
		AudioIndex:    1,
		CropBlackBars: true,

		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second), InputPath: "/media/movie.mkv",
		OutputPath: "/out/clip-1.mp4",

		Status:   clip.StatusPending,
		Progress: 0,
		Error:    "",
	}

	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.MediaTitle, got.MediaTitle)
	assert.Equal(t, "Intro", got.Name)
	assert.Equal(t, 1, got.AudioIndex)
	assert.True(t, got.CropBlackBars)
	assert.False(t, got.WebSafeColor)
	assert.False(t, got.PreserveHDR,
		"a clip saved before the column existed must tone map, as it always did")

	job.WebSafeColor = true
	job.PreserveHDR = true
	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err = db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.True(t, got.WebSafeColor)
	assert.True(t, got.PreserveHDR)

	job.PreserveHDR = false
	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err = db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.True(t, got.WebSafeColor)
	assert.False(t, got.PreserveHDR)

	byMedia, err := db.ListClipsForMedia(t.Context(), "100")
	require.NoError(t, err)
	require.Len(t, byMedia, 1)
	assert.Equal(t, clip.StatusPending, got.Status)

	pending, err := db.ListPendingClips(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)

	job.Status = clip.StatusCompleted
	job.Progress = 100
	require.NoError(t, db.SaveClip(t.Context(), job))

	pending, err = db.ListPendingClips(t.Context())
	require.NoError(t, err)
	assert.Empty(t, pending)

	all, err := db.ListClips(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, clip.StatusCompleted, all[0].Status)

	require.NoError(t, db.DeleteClip(t.Context(), job.ID))

	_, err = db.GetClip(t.Context(), job.ID)
	require.ErrorIs(t, err, ErrClipNotFound)
}

func TestSaveClipRoundTripsTimings(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips-timings.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := testStoredClip("timing-1", "100", time.Now().UTC().Truncate(time.Second))

	job.StartTime = 1500 * time.Millisecond
	job.Duration = 7500 * time.Millisecond
	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, 1500*time.Millisecond, got.StartTime)
	assert.Equal(t, 7500*time.Millisecond, got.Duration)

	var startSeconds float64

	require.NoError(t, db.conn.QueryRowContext(
		t.Context(),
		db.rewrite(`SELECT start_time FROM clips WHERE id = ?`),
		job.ID,
	).Scan(&startSeconds))
	assert.InDelta(t, 1.5, startSeconds, 0.0001)
}

func TestScanJobConvertsSecondsColumnsThroughTimecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give float64
		want time.Duration
	}{
		{give: 0, want: 0},
		{give: 42, want: 42 * time.Second},
		{give: 1.5, want: 1500 * time.Millisecond},
		{give: 7.5, want: 7500 * time.Millisecond},
		{give: 0.006, want: 6 * time.Millisecond},
		{give: 0.0000006, want: 600 * time.Nanosecond},
	}

	for _, tt := range tests {
		assert.Equal(
			t,
			tt.want,
			timecode.FromSeconds(tt.give).Duration(),
			"give %v",
			tt.give,
		)
	}
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

func testStoredClip(id, mediaID string, created time.Time) *clip.Job {
	return &clip.Job{
		ID:         id,
		Type:       clip.TypeClip,
		Name:       id,
		MediaID:    mediaID,
		MediaTitle: "Order Movie",
		MediaType:  "show",

		StartTime:     10 * time.Second,
		Duration:      15 * time.Second,
		Quality:       "high",
		Width:         0,
		FPS:           0,
		AudioIndex:    0,
		CropBlackBars: false,

		CreatedAt: created,
		UpdatedAt: created, InputPath: "/media/order.mkv",
		OutputPath: "/out/" + id + ".mp4",

		Status:   clip.StatusCompleted,
		Progress: 100,
		Error:    "",
	}
}

func TestSaveClipUpdatesGIFDimensionsOnConflict(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips-gif.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := testStoredClip("gif-1", "100", time.Now().UTC().Truncate(time.Second))

	job.Type = clip.TypeGIF
	job.Width = 1280
	job.FPS = 24
	require.NoError(t, db.SaveClip(t.Context(), job))

	job.Width = 720
	job.FPS = 12
	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err := db.GetClip(t.Context(), "gif-1")
	require.NoError(t, err)
	assert.Equal(t, 720, got.Width, "an edited GIF width must survive a resave")
	assert.Equal(t, 12, got.FPS, "an edited GIF frame rate must survive a resave")
}

func TestSaveClipKeepsZeroDimensionsForVideoClips(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips-video.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := testStoredClip("clip-1", "100", time.Now().UTC().Truncate(time.Second))

	job.Width = 0
	job.FPS = 0
	require.NoError(t, db.SaveClip(t.Context(), job))

	job.Name = "Renamed"
	job.StartTime = 42 * time.Second
	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err := db.GetClip(t.Context(), "clip-1")
	require.NoError(t, err)
	assert.Equal(t, 0, got.Width)
	assert.Equal(t, 0, got.FPS)
	assert.Equal(t, "Renamed", got.Name)
	assert.Equal(t, 42*time.Second, got.StartTime)
}

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
	job.MediaType = "movie"
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

func clipIDs(jobs []*clip.Job) []string {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}

	return ids
}

func TestScanJobReadsEveryColumn(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/scan.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	stamp := time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)

	want := &clip.Job{
		ID:         "scan-1",
		Type:       clip.TypeGIF,
		Name:       "Loop",
		MediaID:    "100",
		MediaTitle: "Test Movie",
		MediaType:  "movie",

		StartTime:     12 * time.Second,
		Duration:      7 * time.Second,
		Quality:       "high",
		Width:         640,
		FPS:           24,
		AudioIndex:    2,
		CropBlackBars: true,
		WebSafeColor:  true,
		PreserveHDR:   true,

		CreatedAt: stamp,
		UpdatedAt: stamp.Add(time.Minute), InputPath: "/media/movie.mkv",
		OutputPath: "/out/scan-1.gif",

		Status:   clip.StatusCompleted,
		Progress: 100,
		Error:    "warned",
	}

	require.NoError(t, db.SaveClip(t.Context(), want))

	row := db.conn.QueryRowContext(
		t.Context(),
		db.rewrite(`SELECT `+clipSelectCols+` FROM clips WHERE id = ?`),
		want.ID,
	)

	got, err := scanJob(row)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestScanJobReportsAnUnusableRow(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/scan-bad.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	row := db.conn.QueryRowContext(
		t.Context(),
		db.rewrite(`SELECT `+clipSelectCols+` FROM clips WHERE id = ?`),
		"missing",
	)

	_, err = scanJob(row)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestLegacyTokenAndServerPersistence(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/tokens.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	seedLegacyToken(t, db, "client-a", "token-1")
	seedLegacyToken(t, db, "client-b", "token-2")

	token, err := db.LegacyToken(t.Context())
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

	require.NoError(t, db.ClearLegacyTokens(t.Context()))

	token, err = db.LegacyToken(t.Context())
	require.NoError(t, err)
	assert.Empty(t, token)

	_, ok, err = db.SelectedServer(t.Context())
	require.NoError(t, err)
	assert.True(t, ok, "clearing the legacy tokens keeps the selected server")

	require.NoError(t, db.ClearSelectedServer(t.Context()))

	_, ok, err = db.SelectedServer(t.Context())
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMigrateIdempotent(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/repeat.db"

	db, err := New(path)
	require.NoError(t, err)
	require.NoError(t, db.SaveSetting(t.Context(), "probe", "kept"))
	require.NoError(t, db.Close())

	db, err = New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	value, found, err := db.Setting(t.Context(), "probe")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "kept", value)
}

// seedLegacyToken inserts a row into plex_tokens.
//
// Parameters:
//   - t: The test that owns the database.
//   - db: Database to seed.
//   - clientID: Client identifier the token was issued to.
//   - token: Plex access token to store.
func seedLegacyToken(t *testing.T, db *DB, clientID, token string) {
	t.Helper()

	_, err := db.conn.ExecContext(t.Context(), db.rewrite(`
		INSERT INTO plex_tokens (client_id, access_token, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`), clientID, token)
	require.NoError(t, err)
}
