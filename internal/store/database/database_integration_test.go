// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// clipDatabase returns a migrated SQLite database rooted in the test's temp directory.
//
// Parameters:
//   - t: The test that needs the database.
//
// Returns:
//   - db: A migrated database that is closed when the test finishes.
func clipDatabase(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.New(filepath.Join(t.TempDir(), "outtake.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// clipAt returns a pending clip stamped with the given identity and creation time.
//
// Parameters:
//   - id: Clip identifier.
//   - mediaID: Identifier of the source media item.
//   - createdAt: Creation time the row is stored with.
//
// Returns:
//   - job: A clip ready to persist.
func clipAt(id, mediaID string, createdAt time.Time) *clip.Job {
	return &clip.Job{
		ID:         id,
		Type:       clip.TypeClip,
		Name:       id,
		MediaID:    mediaID,
		MediaTitle: "Integration Movie",
		MediaType:  clip.DefaultMediaType,

		Quality:   "medium",
		StartTime: 2 * time.Second,
		Duration:  6 * time.Second,

		CreatedAt: createdAt,
		UpdatedAt: createdAt, InputPath: "/media/integration.mkv",

		Status: clip.StatusPending,
	}
}

// clipIDs returns the identifiers of clips in the order they were given.
//
// Parameters:
//   - jobs: Clips to read.
//
// Returns:
//   - ids: The identifiers, in order.
func clipIDs(jobs []*clip.Job) []string {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}

	return ids
}

func TestIntegration_ClipRoundTripPersistsEveryColumn(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	base := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	job := clipAt("clip-a", "100", base)

	job.OutputPath = ""
	job.Width = 1280
	job.FPS = 24
	job.AudioIndex = 3
	job.CropBlackBars = true
	job.WebSafeColor = true
	job.PreserveHDR = true
	job.StartTime = 1500 * time.Millisecond
	job.Duration = 7500 * time.Millisecond
	job.Error = "boom"

	require.NoError(t, db.SaveClip(t.Context(), job))

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)

	assert.Equal(t, job.ID, stored.ID)
	assert.Equal(t, "100", stored.MediaID)
	assert.Equal(t, "Integration Movie", stored.MediaTitle)
	assert.Equal(t, clip.DefaultMediaType, stored.MediaType)
	assert.Equal(t, clip.TypeClip, stored.Type)
	assert.Equal(t, clip.StatusPending, stored.Status)
	assert.Equal(t, "/media/integration.mkv", stored.InputPath)
	assert.Equal(t, "boom", stored.Error)
	assert.Equal(t, 1280, stored.Width)
	assert.Equal(t, 24, stored.FPS)
	assert.Equal(t, 3, stored.AudioIndex)
	assert.Equal(t, "medium", stored.Quality)
	assert.Equal(t, 1500*time.Millisecond, stored.StartTime)
	assert.Equal(t, 7500*time.Millisecond, stored.Duration)
	assert.True(t, stored.CropBlackBars, "a true flag survives as a non-zero column")
	assert.True(t, stored.WebSafeColor, "a true flag survives as a non-zero column")
	assert.True(t, stored.PreserveHDR, "a true flag survives as a non-zero column")
	assert.True(t, stored.CreatedAt.Equal(base), "the stored creation time reads back")
	assert.True(t, stored.UpdatedAt.Equal(base), "the stored update time reads back")
}

func TestIntegration_SaveClipUpsertsMutableColumnsAndKeepsIdentity(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	base := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	job := clipAt("clip-a", "100", base)
	require.NoError(t, db.SaveClip(t.Context(), job))

	later := base.Add(time.Hour)

	job.MediaID = "200"
	job.InputPath = "/media/moved.mkv"
	job.Status = clip.StatusCompleted
	job.Progress = 100
	job.Name = "renamed"
	job.CreatedAt = later
	job.UpdatedAt = later

	require.NoError(t, db.SaveClip(t.Context(), job))

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)

	assert.Equal(t, "100", stored.MediaID,
		"the source item is part of a clip's identity, so an upsert cannot move it")
	assert.Equal(t, "/media/integration.mkv", stored.InputPath, "nor the input it was cut from")
	assert.True(t, stored.CreatedAt.Equal(base), "nor when it was created")

	assert.Equal(t, "renamed", stored.Name)
	assert.Equal(t, clip.StatusCompleted, stored.Status)
	assert.Equal(t, 100, stored.Progress)
	assert.True(t, stored.UpdatedAt.Equal(later), "the update time always moves forward")

	byMedia, err := db.ListClipsForMedia(t.Context(), "100")
	require.NoError(t, err)
	require.Len(t, byMedia, 1)
	assert.Equal(t, job.ID, byMedia[0].ID)
}

func TestIntegration_ListClipsOrdersNewestFirstAndBreaksTiesByID(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	base := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

	oldest := clipAt("clip-oldest", "100", base)
	require.NoError(t, db.SaveClip(t.Context(), oldest))

	middle := clipAt("clip-middle", "100", base.Add(time.Minute))
	require.NoError(t, db.SaveClip(t.Context(), middle))

	newest := clipAt("clip-newest", "100", base.Add(2*time.Minute))
	require.NoError(t, db.SaveClip(t.Context(), newest))

	// Two rows stamped at the same instant, so only the id tiebreak can order them.
	tieA := clipAt("clip-tie-a", "100", base.Add(3*time.Minute))
	require.NoError(t, db.SaveClip(t.Context(), tieA))

	tieB := clipAt("clip-tie-b", "100", base.Add(3*time.Minute))
	require.NoError(t, db.SaveClip(t.Context(), tieB))

	all, err := db.ListClips(t.Context())
	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{"clip-tie-b", "clip-tie-a", "clip-newest", "clip-middle", "clip-oldest"},
		clipIDs(all),
	)
}

func TestIntegration_ListPendingClipsKeepsOnlyUnsettledClipsOldestFirst(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	base := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

	pending := clipAt("clip-pending", "100", base.Add(time.Minute))
	require.NoError(t, db.SaveClip(t.Context(), pending))

	processing := clipAt("clip-processing", "100", base)

	processing.Status = clip.StatusProcessing
	require.NoError(t, db.SaveClip(t.Context(), processing))

	completed := clipAt("clip-completed", "100", base.Add(2*time.Minute))

	completed.Status = clip.StatusCompleted
	require.NoError(t, db.SaveClip(t.Context(), completed))

	failed := clipAt("clip-failed", "100", base.Add(3*time.Minute))

	failed.Status = clip.StatusFailed
	require.NoError(t, db.SaveClip(t.Context(), failed))

	canceled := clipAt("clip-canceled", "100", base.Add(4*time.Minute))

	canceled.Status = clip.StatusCancelled
	require.NoError(t, db.SaveClip(t.Context(), canceled))

	outstanding, err := db.ListPendingClips(t.Context())
	require.NoError(t, err)
	assert.Equal(
		t,
		[]string{"clip-processing", "clip-pending"},
		clipIDs(outstanding),
		"processing counts as outstanding and the oldest is picked up first",
	)

	pending.Status = clip.StatusCompleted
	pending.Progress = 100
	require.NoError(t, db.SaveClip(t.Context(), pending))

	outstanding, err = db.ListPendingClips(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"clip-processing"}, clipIDs(outstanding))
}

func TestIntegration_DeletedClipLeavesListClipsAndListClipsForMedia(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	base := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

	first := clipAt("clip-one", "100", base)
	require.NoError(t, db.SaveClip(t.Context(), first))

	second := clipAt("clip-two", "100", base.Add(time.Minute))
	require.NoError(t, db.SaveClip(t.Context(), second))

	other := clipAt("clip-three", "200", base.Add(2*time.Minute))
	require.NoError(t, db.SaveClip(t.Context(), other))

	require.NoError(t, db.DeleteClip(t.Context(), first.ID))

	_, err := db.GetClip(t.Context(), first.ID)
	require.ErrorIs(t, err, database.ErrClipNotFound)

	byMedia, err := db.ListClipsForMedia(t.Context(), "100")
	require.NoError(t, err)
	assert.Equal(t, []string{"clip-two"}, clipIDs(byMedia))

	all, err := db.ListClips(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"clip-three", "clip-two"}, clipIDs(all))

	require.NoError(t, db.DeleteClip(t.Context(), first.ID),
		"deleting an absent clip is not an error")
}

func TestIntegration_DeleteClipProfileKeepsExactlyOneDefault(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	profiles, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)
	require.Len(t, profiles, 3)
	assert.Equal(t, "medium", profiles[0].ID, "the default leads the listing")
	assert.True(t, profiles[0].IsDefault)

	fallback, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "medium", fallback.ID)
	assert.Equal(t, 23, fallback.QualityPreset().CRF)
	assert.Equal(t, 192, fallback.QualityPreset().AudioKbps)
	assert.Equal(t, 1920, fallback.QualityPreset().MaxWidth)

	require.NoError(t, db.SetDefaultClipProfile(t.Context(), "high"))

	afterSet, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "high", afterSet[0].ID)

	moved, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "high", moved.ID)

	require.NoError(t, db.DeleteClipProfile(t.Context(), "high"))

	afterDelete, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)
	require.Len(t, afterDelete, 2)
	assert.True(t, afterDelete[0].IsDefault, "the default leads again after a delete")

	promoted, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, "high", promoted.ID)
}

func TestIntegration_DeletingTheLastClipProfileIsRefused(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	require.NoError(t, db.DeleteClipProfile(t.Context(), "low"))
	require.NoError(t, db.DeleteClipProfile(t.Context(), "high"))

	remaining, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)
	require.Len(t, remaining, 1)

	err = db.DeleteClipProfile(t.Context(), "medium")
	require.ErrorIs(t, err, database.ErrLastClipProfile)

	survivor, err := db.GetClipProfile(t.Context(), "medium")
	require.NoError(t, err)
	assert.True(t, survivor.IsDefault, "the refused delete left the last profile in place")
}

func TestIntegration_SavingAProfileUpdatesItInPlace(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	profile := database.ClipProfile{
		ID:        "custom",
		Name:      "Custom",
		CRF:       20,
		Preset:    "slow",
		AudioKbps: 256,
		MaxWidth:  2560,
		IsDefault: false,
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, db.SaveClipProfile(t.Context(), profile))

	stored, err := db.GetClipProfile(t.Context(), profile.ID)
	require.NoError(t, err)
	assert.Equal(t, profile, stored)
	assert.False(t, stored.IsDefault, "saving a non-default profile never steals the flag")

	profile.CRF = 21
	profile.IsDefault = true

	require.NoError(t, db.SaveClipProfile(t.Context(), profile))

	stored, err = db.GetClipProfile(t.Context(), profile.ID)
	require.NoError(t, err)
	assert.Equal(t, 21, stored.CRF)
	assert.True(t, stored.IsDefault)

	promoted, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "custom", promoted.ID)

	_, err = db.GetClipProfile(t.Context(), "absent")
	require.ErrorIs(t, err, database.ErrClipProfileNotFound)
}

func TestIntegration_LegacyTokenReadsTheMostRecentlyStoredToken(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	token, err := db.LegacyToken(t.Context())
	require.NoError(t, err)
	assert.Empty(t, token, "an empty table is not an error")

	insertLegacyToken(t, db, "client-one", "token-one")
	insertLegacyToken(t, db, "client-two", "token-two")

	token, err = db.LegacyToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "token-two", token, "the newest row wins, by timestamp or by insert order")
}

func TestIntegration_SelectedServerIsASingleRow(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	server, found, err := db.SelectedServer(t.Context())
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, plex.EmptyServer(), server)

	require.NoError(t, db.SaveSelectedServer(t.Context(), plex.Server{
		Name:    "Test Server",
		Address: "127.0.0.1",
		Port:    32400,
		Token:   "server-token",
		Scheme:  "http",
		Local:   true,
	}))

	server, found, err = db.SelectedServer(t.Context())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "Test Server", server.Name)
	assert.Equal(t, "server-token", server.Token)

	require.NoError(t, db.SaveSelectedServer(t.Context(), plex.Server{
		Name:    "Replacement",
		Address: "127.0.0.2",
		Port:    32401,
		Token:   "other-token",
		Scheme:  "http",
	}))

	replaced, found, err := db.SelectedServer(t.Context())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "Replacement", replaced.Name)
	assert.Equal(t, 32401, replaced.Port)
}

func TestIntegration_ResetOwnerReopensTheInstallationToTheNextAccount(t *testing.T) {
	t.Parallel()

	db := clipDatabase(t)

	claimed, err := db.ClaimOwner(t.Context(), 42, "owner")
	require.NoError(t, err)
	require.True(t, claimed)

	insertLegacyToken(t, db, "client-one", "token-one")
	require.NoError(t, db.SaveSelectedServer(t.Context(), plex.Server{
		Name:    "Test Server",
		Address: "127.0.0.1",
		Port:    32400,
		Token:   "server-token",
		Scheme:  "http",
	}))

	removed, err := db.ResetOwner(t.Context())
	require.NoError(t, err)
	assert.True(t, removed)

	_, found, err := db.SelectedServer(t.Context())
	require.NoError(t, err)
	assert.False(t, found, "the selected server went with the owner")

	token, err := db.LegacyToken(t.Context())
	require.NoError(t, err)
	assert.Empty(t, token)

	claimed, err = db.ClaimOwner(t.Context(), 7, "next")
	require.NoError(t, err)
	assert.True(t, claimed, "the next account claims the installation")
}

// insertLegacyToken inserts a row into plex_tokens.
//
// Parameters:
//   - t: The test that owns the database.
//   - db: Database to seed.
//   - clientID: Client identifier the token was issued to.
//   - token: Plex access token to store.
func insertLegacyToken(t *testing.T, db *database.DB, clientID, token string) {
	t.Helper()

	_, err := db.Conn().ExecContext(t.Context(), `
		INSERT INTO plex_tokens (client_id, access_token, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, clientID, token)
	require.NoError(t, err)
}
