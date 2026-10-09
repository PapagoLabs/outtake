// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// generatedID matches the ids profiles are stored under: 32 lowercase hex
// characters.
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

// TestClipProfilesSeeded covers a fresh database's built-ins: four profiles
// named after what they produce, under generated ids, with 1080p the default
// and only 4K HDR keeping HDR. 1080p matches the in-code default preset.
func TestClipProfilesSeeded(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/profile.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	profiles, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)
	require.Len(t, profiles, 4)

	want := map[string]clip.QualityPreset{
		"720p":   {CRF: 21, Preset: "medium", AudioKbps: 160, MaxWidth: 1280, PreserveHDR: false},
		"1080p":  clip.DefaultPreset,
		"4K":     {CRF: 18, Preset: "slow", AudioKbps: 256, MaxWidth: 3840, PreserveHDR: false},
		"4K HDR": {CRF: 18, Preset: "slow", AudioKbps: 256, MaxWidth: 3840, PreserveHDR: true},
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
