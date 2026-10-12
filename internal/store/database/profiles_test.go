// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// generatedID matches the ids profiles are stored under.
//
// Such an id is 32 lowercase hex characters.
var generatedID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// builtinIDs maps each stored profile's name to its id.
//
// Parameters:
//   - t: The test that needs the ids.
//   - db: A migrated database.
//
// Returns:
//   - ids: Each stored profile's id by name.
func builtinIDs(t *testing.T, db *DB) map[string]string {
	t.Helper()

	profiles, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)

	ids := make(map[string]string, len(profiles))

	for i := range profiles {
		ids[profiles[i].Name] = profiles[i].ID
	}

	return ids
}

// TestClipProfilesSeeded covers a fresh database's built-ins.
//
// There are five profiles named after what they produce, under generated ids.
// 1080p is the default and matches the in-code default preset. 4K HDR and
// Original keep HDR, and only Original copies a source that fits.
func TestClipProfilesSeeded(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/profile.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	profiles, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)
	require.Len(t, profiles, 5)

	want := map[string]clip.QualityPreset{
		"720p": {
			CRF:         21,
			Preset:      "medium",
			AudioKbps:   160,
			MaxWidth:    1280,
			PreserveHDR: false,
			CopyVideo:   false,
		},
		"1080p": clip.DefaultPreset,
		"4K": {
			CRF:         18,
			Preset:      "slow",
			AudioKbps:   256,
			MaxWidth:    3840,
			PreserveHDR: false,
			CopyVideo:   false,
		},
		"4K HDR": {
			CRF:         18,
			Preset:      "slow",
			AudioKbps:   256,
			MaxWidth:    3840,
			PreserveHDR: true,
			CopyVideo:   false,
		},
		"Original": {
			CRF:         18,
			Preset:      "slow",
			AudioKbps:   256,
			MaxWidth:    3840,
			PreserveHDR: true,
			CopyVideo:   true,
		},
	}

	for i := range profiles {
		profile := profiles[i]

		assert.Regexp(t, generatedID, profile.ID, profile.Name)
		assert.Equal(t, want[profile.Name], profile.QualityPreset(), profile.Name)
		assert.Equal(t, profile.Name == "1080p", profile.IsDefault, profile.Name)
	}

	assert.Equal(t, "1080p", profiles[0].Name, "the default leads the listing")
}

func TestClipProfileCRUD(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/profiles-crud.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC().Truncate(time.Second)
	custom := ClipProfile{
		ID:        "custom-1",
		Name:      "Archive",
		CRF:       20,
		Preset:    "slow",
		AudioKbps: 320,
		MaxWidth:  clip.OutputWidth2160p,
		IsDefault: true,
		KeepHDR:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, db.SaveClipProfile(t.Context(), custom))

	got, err := db.GetClipProfile(t.Context(), custom.ID)
	require.NoError(t, err)
	assert.Equal(t, "Archive", got.Name)
	assert.True(t, got.KeepHDR)
	assert.Equal(t, 320, got.AudioKbps)
	assert.Equal(t, clip.OutputWidth2160p, got.MaxWidth)
	assert.True(t, got.IsDefault)

	ids := builtinIDs(t, db)

	builtIn, err := db.GetClipProfile(t.Context(), ids["1080p"])
	require.NoError(t, err)
	assert.False(t, builtIn.IsDefault)

	require.NoError(t, db.SetDefaultClipProfile(t.Context(), ids["4K"]))

	fourK, err := db.GetClipProfile(t.Context(), ids["4K"])
	require.NoError(t, err)
	assert.True(t, fourK.IsDefault)

	custom, err = db.GetClipProfile(t.Context(), custom.ID)
	require.NoError(t, err)
	assert.False(t, custom.IsDefault)

	require.NoError(t, db.DeleteClipProfile(t.Context(), custom.ID))

	_, err = db.GetClipProfile(t.Context(), custom.ID)
	require.ErrorIs(t, err, ErrClipProfileNotFound)
}

func TestDeleteLastClipProfile(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/profiles-last.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	ids := builtinIDs(t, db)

	require.NoError(t, db.DeleteClipProfile(t.Context(), ids["720p"]))
	require.NoError(t, db.DeleteClipProfile(t.Context(), ids["4K"]))
	require.NoError(t, db.DeleteClipProfile(t.Context(), ids["4K HDR"]))
	require.NoError(t, db.DeleteClipProfile(t.Context(), ids["Original"]))

	err = db.DeleteClipProfile(t.Context(), ids["1080p"])
	require.ErrorIs(t, err, ErrLastClipProfile)

	def, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.Equal(t, ids["1080p"], def.ID)
	assert.True(t, def.IsDefault)
}

func TestDeleteDefaultPromotesAnother(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/profiles-promote.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	ids := builtinIDs(t, db)

	require.NoError(t, db.DeleteClipProfile(t.Context(), ids["1080p"]))

	def, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.True(t, def.IsDefault)
	assert.NotEqual(t, ids["1080p"], def.ID)
}

// TestSaveClipProfileRefusesADuplicateName covers the unique profile name.
//
// Saving a second profile under a name another profile has is refused with
// ErrDuplicateClipProfileName, read from the driver's constraint code. The
// first profile stays as it was.
func TestSaveClipProfileRefusesADuplicateName(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/profiles-duplicate.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC().Truncate(time.Second)

	err = db.SaveClipProfile(t.Context(), ClipProfile{
		ID:        "copy-of-1080p",
		Name:      "1080p",
		CRF:       24,
		Preset:    "fast",
		AudioKbps: 128,
		MaxWidth:  clip.OutputWidth1080p,
		IsDefault: false,
		KeepHDR:   false,
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.ErrorIs(t, err, ErrDuplicateClipProfileName)

	builtIn, err := db.GetClipProfile(t.Context(), builtinIDs(t, db)["1080p"])
	require.NoError(t, err)
	assert.Equal(t, 20, builtIn.CRF, "the profile that had the name is untouched")

	_, err = db.GetClipProfile(t.Context(), "copy-of-1080p")
	require.ErrorIs(t, err, ErrClipProfileNotFound)
}

// TestIsUniqueViolationNeedsADriverCode covers the unique violation check
// itself.
//
// An error without a driver's constraint code, such as a libsql one, is not
// read as a duplicate.
func TestIsUniqueViolationNeedsADriverCode(t *testing.T) {
	t.Parallel()

	assert.False(t, isUniqueViolation(nil))
	assert.False(t, isUniqueViolation(ErrClipProfileNotFound))
	assert.True(t, isUniqueViolation(&pgconn.PgError{Code: pgUniqueViolation}))
	assert.False(t, isUniqueViolation(&pgconn.PgError{Code: "23503"}))
}

// TestMigrationCopyProfilesKeepsACustomOriginal covers a database that
// already has a profile named Original when migration 012 runs.
//
// Names are unique, so the custom profile stays as it was and stands in for
// the built-in, and no second Original appears.
func TestMigrationCopyProfilesKeepsACustomOriginal(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/custom-original.db"

	db, err := New(path)
	require.NoError(t, err)

	for _, statement := range []string{
		`DELETE FROM clip_profiles WHERE name = 'Original'`,
		`ALTER TABLE clip_profiles DROP COLUMN copy_video`,
		`DELETE FROM schema_migrations WHERE name = '012_copy_profiles.sql'`,
		`INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width, is_default, keep_hdr)
			VALUES ('mine', 'Original', 23, 'fast', 128, 1280, 0, 0)`,
	} {
		_, err = db.Conn().ExecContext(t.Context(), statement)
		require.NoError(t, err, statement)
	}

	require.NoError(t, db.Close())

	db, err = New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	profiles, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)

	originals := 0

	for i := range profiles {
		if profiles[i].Name == "Original" {
			originals++

			assert.Equal(t, "mine", profiles[i].ID, "the custom profile keeps its id")
			assert.Equal(t, 23, profiles[i].CRF, "and its settings")
			assert.False(t, profiles[i].CopyVideo, "and does not start copying")
		}
	}

	assert.Equal(t, 1, originals)
}

// TestSaveClipProfileKeepsCopyVideo covers the copy setting through a save
// and a read.
func TestSaveClipProfileKeepsCopyVideo(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/copy-video.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, db.SaveClipProfile(t.Context(), ClipProfile{
		ID: "copier", Name: "Copier", CRF: 20, Preset: "medium", AudioKbps: 192, MaxWidth: 1920,
		IsDefault: false, KeepHDR: false, CopyVideo: true, CreatedAt: now, UpdatedAt: now,
	}))

	stored, err := db.GetClipProfile(t.Context(), "copier")
	require.NoError(t, err)
	assert.True(t, stored.CopyVideo)
	assert.True(t, stored.QualityPreset().CopyVideo, "the preset a render reads carries it")
}
