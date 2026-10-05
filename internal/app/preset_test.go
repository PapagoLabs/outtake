// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

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
	CRF:       0,
	Preset:    "",
	AudioKbps: 0,
	MaxWidth:  0,
}

// testDatabase opens a migrated database inside the test's temporary directory.
//
// Parameters:
//   - t: The test that needs the database.
//
// Returns:
//   - db: The opened database.
func testDatabase(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.New(filepath.Join(t.TempDir(), "app-preset.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}

// closedDatabase opens a database and closes it again so its reads fail.
//
// Parameters:
//   - t: The test that needs the closed database.
//
// Returns:
//   - db: The closed database handle.
func closedDatabase(t *testing.T) *database.DB {
	t.Helper()

	db := testDatabase(t)
	require.NoError(t, db.Close())

	return db
}

// storeTestProfile writes the profile the preset tests resolve.
//
// Parameters:
//   - t: The test that needs the profile.
//   - db: Database the profile is written to.
func storeTestProfile(t *testing.T, db *database.DB) {
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
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
}

func TestClipEncodePresetAppliesTheClipsColorDecisions(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	storeTestProfile(t, db)

	job := &clip.Job{
		Quality:       "archive",
		WebSafeColor:  true,
		PreserveHDR:   true,
		CropBlackBars: false,
	}

	preset := clipEncodePreset(t.Context(), db, &job.Clip)

	assert.Equal(t, 20, preset.CRF)
	assert.Equal(t, "slow", preset.Preset)
	assert.True(t, preset.WebSafeColor, "the clip decides whether color is remapped")
	assert.True(t, preset.PreserveHDR, "the clip decides whether the source stays HDR")
}

func TestClipEncodePresetKeepsTheStoredPresetUncolored(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	storeTestProfile(t, db)

	job := &clip.Job{Quality: "archive"}

	preset := clipEncodePreset(t.Context(), db, &job.Clip)

	assert.False(t, preset.WebSafeColor, "a clip that asked for nothing gets no remapping")
	assert.False(t, preset.PreserveHDR, "a clip that asked for nothing gets no preservation")
}

func TestClipPresetEmptyQualityUsesTheDefaultProfile(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	storeTestProfile(t, db)

	preset := clipPreset(t.Context(), db, "")

	assert.Equal(t, 20, preset.CRF)
	assert.Equal(t, "slow", preset.Preset)
	assert.Equal(t, clip.OutputWidth2160p, preset.MaxWidth)
}

func TestClipPresetResolvesABuiltInQuality(t *testing.T) {
	t.Parallel()

	preset := clipPreset(t.Context(), nil, string(clip.ClipQualityHigh))

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityHigh].CRF, preset.CRF)
	assert.Equal(t, clip.QualityPresets[clip.ClipQualityHigh].MaxWidth, preset.MaxWidth)
}

func TestClipPresetUnknownQualityFallsBackToMedium(t *testing.T) {
	t.Parallel()

	preset := clipPreset(t.Context(), testDatabase(t), "no-such-quality")

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium].CRF, preset.CRF)
	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium].Preset, preset.Preset)
}

func TestClipPresetStoredProfileIsNormalized(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

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

	preset := clipPreset(t.Context(), db, "archive")

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium], preset,
		"an out of range profile is replaced by the Medium built-in")
}

func TestDefaultClipPresetWithoutADatabaseIsMedium(t *testing.T) {
	t.Parallel()

	preset := defaultClipPreset(t.Context(), nil)

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium], preset)
}

func TestDefaultClipPresetReadsTheStoredProfile(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

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

	preset := defaultClipPreset(t.Context(), db)

	assert.Equal(t, 20, preset.CRF)
	assert.Equal(t, clip.OutputWidth1440p, preset.MaxWidth)
}

func TestDefaultClipPresetFallsBackWhenTheSeededProfileIsMissing(t *testing.T) {
	t.Parallel()

	preset := defaultClipPreset(t.Context(), testDatabase(t))

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium].CRF, preset.CRF,
		"a fresh database falls back to the seeded Medium profile")
}

func TestDefaultClipPresetReportsAReadFailureAsMedium(t *testing.T) {
	t.Parallel()

	preset := defaultClipPreset(t.Context(), closedDatabase(t))

	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium], preset)
}

func TestLookupClipPresetFindsAStoredProfile(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	storeTestProfile(t, db)

	preset, found := lookupClipPreset(t.Context(), db, "archive")

	assert.True(t, found)
	assert.Equal(t, 20, preset.CRF)
	assert.Equal(t, "slow", preset.Preset)
	assert.Equal(t, 320, preset.AudioKbps)
	assert.Equal(t, clip.OutputWidth2160p, preset.MaxWidth)
	assert.False(t, preset.WebSafeColor, "a stored profile carries no color decision")
}

func TestLookupClipPresetWithoutADatabaseMisses(t *testing.T) {
	t.Parallel()

	preset, found := lookupClipPreset(t.Context(), nil, "archive")

	assert.False(t, found)
	assert.Equal(t, emptyPreset, preset)
}

func TestLookupClipPresetMissesAnUnknownID(t *testing.T) {
	t.Parallel()

	preset, found := lookupClipPreset(t.Context(), testDatabase(t), "no-such-profile")

	assert.False(t, found)
	assert.Equal(t, emptyPreset, preset)
}

func TestLookupClipPresetReportsAReadFailureAsAMiss(t *testing.T) {
	t.Parallel()

	preset, found := lookupClipPreset(t.Context(), closedDatabase(t), "archive")

	assert.False(t, found)
	assert.Equal(t, emptyPreset, preset)
}
