// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/media"
)

func TestClipProfilesSeeded(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/profiles.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	profiles, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)
	require.Len(t, profiles, 3)
	assert.Equal(t, "medium", profiles[0].ID)
	assert.True(t, profiles[0].IsDefault)

	def, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "medium", def.ID)
	assert.Equal(t, media.QualityPresets[media.ClipQualityMedium].CRF, def.CRF)
	assert.Equal(t, media.QualityPresets[media.ClipQualityMedium].AudioKbps, def.AudioKbps)
	assert.Equal(t, media.OutputWidth1080p, def.MaxWidth)

	high, err := db.GetClipProfile(t.Context(), "high")
	require.NoError(t, err)
	assert.Equal(t, media.OutputWidth2160p, high.MaxWidth)
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
		MaxWidth:  media.OutputWidth2160p,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, db.SaveClipProfile(t.Context(), custom))

	got, err := db.GetClipProfile(t.Context(), custom.ID)
	require.NoError(t, err)
	assert.Equal(t, "Archive", got.Name)
	assert.Equal(t, 320, got.AudioKbps)
	assert.Equal(t, media.OutputWidth2160p, got.MaxWidth)
	assert.True(t, got.IsDefault)

	medium, err := db.GetClipProfile(t.Context(), "medium")
	require.NoError(t, err)
	assert.False(t, medium.IsDefault)

	require.NoError(t, db.SetDefaultClipProfile(t.Context(), "high"))

	high, err := db.GetClipProfile(t.Context(), "high")
	require.NoError(t, err)
	assert.True(t, high.IsDefault)

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

	require.NoError(t, db.DeleteClipProfile(t.Context(), "low"))
	require.NoError(t, db.DeleteClipProfile(t.Context(), "high"))

	err = db.DeleteClipProfile(t.Context(), "medium")
	require.ErrorIs(t, err, ErrLastClipProfile)

	def, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "medium", def.ID)
	assert.True(t, def.IsDefault)
}

func TestDeleteDefaultPromotesAnother(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/profiles-promote.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.DeleteClipProfile(t.Context(), "medium"))

	def, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.True(t, def.IsDefault)
	assert.NotEqual(t, "medium", def.ID)
}
