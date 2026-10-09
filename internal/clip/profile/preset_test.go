// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// emptyPreset is the zero clip.QualityPreset, which is what a failed lookup
// reports.
var emptyPreset = clip.QualityPreset{
	CRF:         0,
	Preset:      "",
	AudioKbps:   0,
	MaxWidth:    0,
	PreserveHDR: false,
}

// presetDatabase opens a migrated database inside the test's temporary directory.
//
// Parameters:
//   - t: The test that needs the database.
//
// Returns:
//   - db: The opened database.
func presetDatabase(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.New(filepath.Join(t.TempDir(), "preset.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}

// closedPresetDatabase opens a database and closes it again so its reads fail.
//
// Parameters:
//   - t: The test that needs the closed database.
//
// Returns:
//   - db: The closed database handle.
func closedPresetDatabase(t *testing.T) *database.DB {
	t.Helper()

	db := presetDatabase(t)
	require.NoError(t, db.Close())

	return db
}

// storePresetProfile writes the profile the preset tests resolve.
//
// Parameters:
//   - t: The test that needs the profile.
//   - db: Database the profile is written to.
func storePresetProfile(t *testing.T, db *database.DB) {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Second)

	err := db.SaveClipProfile(t.Context(), database.ClipProfile{
		ID:        "archive",
		Name:      "Archive",
		CRF:       20,
		Preset:    "slow",
		AudioKbps: 320,
		MaxWidth:  clip.OutputWidth2160p,
		IsDefault: true,
		KeepHDR:   false,
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
}

func TestPresetEmptyQualityUsesTheDefaultProfile(t *testing.T) {
	t.Parallel()

	db := presetDatabase(t)
	storePresetProfile(t, db)

	preset := Preset(t.Context(), db, "")

	assert.Equal(t, 20, preset.CRF)
	assert.Equal(t, "slow", preset.Preset)
	assert.Equal(t, clip.OutputWidth2160p, preset.MaxWidth)
}

func TestPresetResolvesABuiltInQuality(t *testing.T) {
	t.Parallel()

	preset := Preset(t.Context(), nil, string(clip.ClipQualityHigh))

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityHigh].CRF, preset.CRF)
	assert.Equal(t, clip.QualityPresets[clip.ClipQualityHigh].MaxWidth, preset.MaxWidth)
}

func TestPresetUnknownQualityFallsBackToMedium(t *testing.T) {
	t.Parallel()

	preset := Preset(t.Context(), presetDatabase(t), "no-such-quality")

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium].CRF, preset.CRF)
	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium].Preset, preset.Preset)
}

func TestPresetStoredProfileIsNormalized(t *testing.T) {
	t.Parallel()

	db := presetDatabase(t)

	now := time.Now().UTC().Truncate(time.Second)

	err := db.SaveClipProfile(t.Context(), database.ClipProfile{
		ID:        "archive",
		Name:      "Archive",
		CRF:       clip.MaxCRF + 1,
		Preset:    "not-a-preset",
		AudioKbps: clip.MinAudioKbps - 1,
		MaxWidth:  1234,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	preset := Preset(t.Context(), db, "archive")

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium], preset,
		"an out of range profile is replaced by the Medium built-in")
}

func TestDefaultPresetWithoutADatabaseIsMedium(t *testing.T) {
	t.Parallel()

	preset := defaultPreset(t.Context(), nil)

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium], preset)
}

func TestDefaultPresetReadsTheStoredProfile(t *testing.T) {
	t.Parallel()

	db := presetDatabase(t)

	now := time.Now().UTC().Truncate(time.Second)

	err := db.SaveClipProfile(t.Context(), database.ClipProfile{
		ID:        "archive",
		Name:      "Archive",
		CRF:       20,
		Preset:    "slow",
		AudioKbps: 320,
		MaxWidth:  clip.OutputWidth1440p,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	preset := defaultPreset(t.Context(), db)

	assert.Equal(t, 20, preset.CRF)
	assert.Equal(t, clip.OutputWidth1440p, preset.MaxWidth)
}

func TestDefaultPresetFallsBackWhenTheSeededProfileIsMissing(t *testing.T) {
	t.Parallel()

	preset := defaultPreset(t.Context(), presetDatabase(t))

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium].CRF, preset.CRF,
		"a fresh database falls back to the seeded Medium profile")
}

func TestDefaultPresetReportsAReadFailureAsMedium(t *testing.T) {
	t.Parallel()

	preset := defaultPreset(t.Context(), closedPresetDatabase(t))

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium], preset)
}

func TestLookupPresetFindsAStoredProfile(t *testing.T) {
	t.Parallel()

	db := presetDatabase(t)
	storePresetProfile(t, db)

	preset, found := lookupPreset(t.Context(), db, "archive")

	assert.True(t, found)
	assert.Equal(t, 20, preset.CRF)
	assert.Equal(t, "slow", preset.Preset)
	assert.Equal(t, 320, preset.AudioKbps)
	assert.Equal(t, clip.OutputWidth2160p, preset.MaxWidth)
	assert.False(t, preset.PreserveHDR, "the profile was stored without keep-HDR")
}

func TestLookupPresetWithoutADatabaseMisses(t *testing.T) {
	t.Parallel()

	preset, found := lookupPreset(t.Context(), nil, "archive")

	assert.False(t, found)
	assert.Equal(t, emptyPreset, preset)
}

func TestLookupPresetMissesAnUnknownID(t *testing.T) {
	t.Parallel()

	preset, found := lookupPreset(t.Context(), presetDatabase(t), "no-such-profile")

	assert.False(t, found)
	assert.Equal(t, emptyPreset, preset)
}

func TestLookupPresetReportsAReadFailureAsAMiss(t *testing.T) {
	t.Parallel()

	preset, found := lookupPreset(t.Context(), closedPresetDatabase(t), "archive")

	assert.False(t, found)
	assert.Equal(t, emptyPreset, preset)
}

// TestPresetCarriesTheProfilesKeepHDR covers the setting a clip takes when it
// renders: a stored profile's Keep HDR, where only High HDR keeps HDR among
// the built-ins, both stored and without a database.
func TestPresetCarriesTheProfilesKeepHDR(t *testing.T) {
	t.Parallel()

	db := presetDatabase(t)

	assert.True(t, Preset(t.Context(), db, "high-hdr").PreserveHDR)
	assert.False(t, Preset(t.Context(), db, "high").PreserveHDR, "High converts to SDR")
	assert.False(t, Preset(t.Context(), db, "medium").PreserveHDR)
	assert.False(t, Preset(t.Context(), db, "").PreserveHDR, "the default profile is Medium")
	assert.True(t, Preset(t.Context(), nil, "high-hdr").PreserveHDR,
		"the built-in High HDR keeps HDR")
	assert.False(t, Preset(t.Context(), nil, "high").PreserveHDR)
}

// TestBuiltinProfilesOfferOneThatKeepsHDR covers the built-ins without a
// database: High HDR is the only one that keeps HDR, and Medium is the default.
func TestBuiltinProfilesOfferOneThatKeepsHDR(t *testing.T) {
	t.Parallel()

	var keeping, defaults []string

	for _, option := range BuiltinProfiles() {
		if option.KeepHDR {
			keeping = append(keeping, option.ID)
		}

		if option.IsDefault {
			defaults = append(defaults, option.ID)
		}
	}

	assert.Equal(t, []string{"high-hdr"}, keeping)
	assert.Equal(t, []string{"medium"}, defaults)
}
