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

func TestClipPersistence(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := &queue.Job{
		ID:            "clip-1",
		Type:          queue.JobTypeClip,
		Name:          "Bond intro",
		MediaID:       "100",
		MediaTitle:    "Test Movie",
		MediaType:     "movie",
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
	assert.Equal(t, "Bond intro", got.Name)
	assert.Equal(t, 1, got.AudioIndex)
	assert.True(t, got.CropBlackBars)

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
