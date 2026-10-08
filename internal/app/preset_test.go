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
		PreserveHDR:   true,
		CropBlackBars: false,
	}

	preset := clipEncodePreset(t.Context(), db, &job.Clip)

	assert.Equal(t, 20, preset.CRF)
	assert.Equal(t, "slow", preset.Preset)
	assert.True(t, preset.PreserveHDR, "the clip decides whether the source stays HDR")
}

func TestClipEncodePresetKeepsTheStoredPresetUncolored(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	storeTestProfile(t, db)

	job := &clip.Job{Quality: "archive"}

	preset := clipEncodePreset(t.Context(), db, &job.Clip)

	assert.False(t, preset.PreserveHDR, "a clip that keeps no HDR tone maps it")
}
