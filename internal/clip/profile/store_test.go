// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/store/database"
)

// profileService builds the clip profile service over a temporary database.
//
// Parameters:
//   - t: The test the service belongs to.
//
// Returns:
//   - profiles: The service under test.
//   - store: The database behind it.
func profileService(t *testing.T) (*Service, *database.DB) {
	t.Helper()

	store, err := database.New(filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = store.Close() })

	return New(store), store
}

func TestProfilesCreateStoresAValidatedProfile(t *testing.T) {
	t.Parallel()

	profiles, store := profileService(t)

	profile, err := profiles.Create(t.Context(), fieldsWith(t, func(f *ProfileFields) {
		f.IsDefault = true
	}))
	require.NoError(t, err)

	assert.NotEmpty(t, profile.ID)
	assert.Equal(t, "Archive", profile.Name)
	assert.Equal(t, 18, profile.CRF)
	assert.True(t, profile.IsDefault)

	stored, err := store.GetClipProfile(t.Context(), profile.ID)
	require.NoError(t, err)
	assert.Equal(t, profile.Name, stored.Name)
	assert.Equal(t, 3840, stored.MaxWidth)
}

func TestProfilesCreateRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	profiles, _ := profileService(t)

	before := len(profiles.List(t.Context()))

	_, err := profiles.Create(t.Context(), fieldsWith(t, func(f *ProfileFields) {
		f.CRF = "99"
	}))
	require.ErrorIs(t, err, errProfileCRF)

	assert.Len(t, profiles.List(t.Context()), before,
		"a rejected form stores nothing")
}

func TestProfilesUpdateKeepsTheCreationStampAndDefaultFlag(t *testing.T) {
	t.Parallel()

	profiles, store := profileService(t)

	created, err := profiles.Create(t.Context(), fieldsWith(t, func(f *ProfileFields) {
		f.IsDefault = true
	}))
	require.NoError(t, err)

	updated, err := profiles.Update(t.Context(), created.ID, fieldsWith(t, func(f *ProfileFields) {
		f.Name = "Renamed"
		f.CRF = "20"
		// A form cannot promote a profile, so this is ignored.
		f.IsDefault = false
	}))
	require.NoError(t, err)

	assert.Equal(t, "Renamed", updated.Name)
	assert.Equal(t, 20, updated.CRF)
	assert.True(t, updated.IsDefault, "an edit cannot take the default away")
	assert.Equal(t, created.CreatedAt, updated.CreatedAt)

	stored, err := store.GetClipProfile(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", stored.Name)
	assert.True(t, stored.IsDefault)
}

func TestProfilesUpdateReportsAnUnknownProfile(t *testing.T) {
	t.Parallel()

	profiles, _ := profileService(t)

	_, err := profiles.Update(t.Context(), "no-such-profile", archiveFields())
	require.ErrorIs(t, err, database.ErrClipProfileNotFound)
}

func TestProfilesSetDefaultAndDelete(t *testing.T) {
	t.Parallel()

	profiles, _ := profileService(t)

	first, err := profiles.Create(t.Context(), archiveFields())
	require.NoError(t, err)

	_, err = profiles.Create(t.Context(), fieldsWith(t, func(f *ProfileFields) {
		f.Name = "Draft"
	}))
	require.NoError(t, err)

	require.NoError(t, profiles.SetDefault(t.Context(), first.ID))

	listed := profiles.List(t.Context())
	require.NotEmpty(t, listed)
	assert.True(t, listed[0].IsDefault, "the default profile leads the list")
	assert.Equal(t, first.ID, listed[0].ID)

	require.NoError(t, profiles.Delete(t.Context(), first.ID))
	assert.NotContains(t, profiles.List(t.Context()), Profile{ID: first.ID, IsDefault: true},
		"the removed profile is gone, though built-ins seed the table")
}

func TestProfilesDeleteReportsAFailedRemoval(t *testing.T) {
	t.Parallel()

	profiles, _ := profileService(t)

	err := profiles.Delete(t.Context(), "no-such-profile")
	require.Error(t, err)
}
