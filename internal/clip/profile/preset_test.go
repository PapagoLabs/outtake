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

// builtinID finds the generated id of a built-in profile the migrations
// stored.
//
// Parameters:
//   - t: The test that needs the id.
//   - db: A migrated database.
//   - name: The built-in's name, such as "4K HDR".
//
// Returns:
//   - id: The stored profile's id.
func builtinID(t *testing.T, db *database.DB, name string) string {
	t.Helper()

	profiles, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)

	for i := range profiles {
		if profiles[i].Name == name {
			return profiles[i].ID
		}
	}

	require.Failf(t, "built-in profile missing", "no stored profile is named %q", name)

	return ""
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

// TestPresetResolvesAStoredBuiltIn covers a built-in, found by the id the
// migrations generated for it.
func TestPresetResolvesAStoredBuiltIn(t *testing.T) {
	t.Parallel()

	db := presetDatabase(t)

	preset := Preset(t.Context(), db, builtinID(t, db, "4K"))

	assert.Equal(t, 18, preset.CRF)
	assert.Equal(t, clip.OutputWidth2160p, preset.MaxWidth)
}

// TestPresetUnknownQualityFallsBackToTheDefaultPreset covers an id no stored
// profile has, including an old built-in id: it renders with the in-code
// 1080p settings, without a database too.
func TestPresetUnknownQualityFallsBackToTheDefaultPreset(t *testing.T) {
	t.Parallel()

	assert.Equal(t, clip.DefaultPreset, Preset(t.Context(), presetDatabase(t), "no-such-quality"))
	assert.Equal(t, clip.DefaultPreset, Preset(t.Context(), presetDatabase(t), "high"))
	assert.Equal(t, clip.DefaultPreset, Preset(t.Context(), nil, "high"))
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

	assert.Equal(t, clip.DefaultPreset, preset,
		"an out of range profile is replaced by the default preset")
}

func TestDefaultPresetWithoutADatabaseIsTheInCodePreset(t *testing.T) {
	t.Parallel()

	assert.Equal(t, clip.DefaultPreset, defaultPreset(t.Context(), nil))
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

// TestDefaultPresetOfAFreshDatabaseIs1080p covers the default the migrations
// store: the 1080p built-in.
func TestDefaultPresetOfAFreshDatabaseIs1080p(t *testing.T) {
	t.Parallel()

	assert.Equal(t, clip.DefaultPreset, defaultPreset(t.Context(), presetDatabase(t)),
		"a fresh database's default is the stored 1080p profile")
}

func TestDefaultPresetReportsAReadFailureAsTheInCodePreset(t *testing.T) {
	t.Parallel()

	assert.Equal(t, clip.DefaultPreset, defaultPreset(t.Context(), closedPresetDatabase(t)))
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
// renders: a stored profile's Keep HDR, where only 4K HDR keeps HDR among the
// built-ins, and the default 1080p converts to SDR.
func TestPresetCarriesTheProfilesKeepHDR(t *testing.T) {
	t.Parallel()

	db := presetDatabase(t)

	assert.True(t, Preset(t.Context(), db, builtinID(t, db, "4K HDR")).PreserveHDR)
	assert.False(t, Preset(t.Context(), db, builtinID(t, db, "4K")).PreserveHDR,
		"4K converts to SDR")
	assert.False(t, Preset(t.Context(), db, builtinID(t, db, "1080p")).PreserveHDR)
	assert.False(t, Preset(t.Context(), db, builtinID(t, db, "720p")).PreserveHDR)
	assert.False(t, Preset(t.Context(), db, "").PreserveHDR, "the default profile is 1080p")
}

// TestFallbackProfilesOfferTheDefault covers the quality select without stored
// profiles: one option, with the empty id every caller resolves to the
// default profile, named after the in-code preset.
func TestFallbackProfilesOfferTheDefault(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []ProfileOption{{ID: "", Name: "1080p", IsDefault: true, KeepHDR: false}},
		FallbackProfiles())
}

// TestKeepHDRReportsAProfileThatIsGone covers the lookup an edit uses: a
// stored profile and the default profile are found, and an id no stored
// profile has, such as an old built-in id, is reported missing rather than
// answered with the default preset's setting.
func TestKeepHDRReportsAProfileThatIsGone(t *testing.T) {
	t.Parallel()

	db := presetDatabase(t)

	keep, found := KeepHDR(t.Context(), db, builtinID(t, db, "4K HDR"))
	assert.True(t, found)
	assert.True(t, keep)

	keep, found = KeepHDR(t.Context(), db, builtinID(t, db, "4K"))
	assert.True(t, found)
	assert.False(t, keep)

	keep, found = KeepHDR(t.Context(), db, "")
	assert.True(t, found, "the default profile")
	assert.False(t, keep)

	_, found = KeepHDR(t.Context(), db, "deleted-profile")
	assert.False(t, found)

	_, found = KeepHDR(t.Context(), db, "high-hdr")
	assert.False(t, found, "an old built-in id names no profile")

	_, found = KeepHDR(t.Context(), nil, "high-hdr")
	assert.False(t, found, "without a database no id is found")

	keep, found = KeepHDR(t.Context(), nil, "")
	assert.True(t, found, "without a database the default is the in-code preset")
	assert.False(t, keep)
}
