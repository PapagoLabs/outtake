// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
)

func TestClipProfilesSeeded(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/profile.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	profiles, err := db.ListClipProfiles(t.Context())
	require.NoError(t, err)
	require.Len(t, profiles, 4)
	assert.Equal(t, "medium", profiles[0].ID)
	assert.True(t, profiles[0].IsDefault)

	def, err := db.DefaultClipProfile(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "medium", def.ID)
	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium].CRF, def.CRF)
	assert.Equal(t, clip.QualityPresets[clip.ClipQualityMedium].AudioKbps, def.AudioKbps)
	assert.Equal(t, clip.OutputWidth1080p, def.MaxWidth)

	high, err := db.GetClipProfile(t.Context(), "high")
	require.NoError(t, err)
	assert.Equal(t, clip.OutputWidth2160p, high.MaxWidth)
	assert.False(t, high.KeepHDR, "High converts to SDR")

	highHDR, err := db.GetClipProfile(t.Context(), "high-hdr")
	require.NoError(t, err)
	assert.Equal(t, "High HDR", highHDR.Name)
	assert.Equal(t, clip.OutputWidth2160p, highHDR.MaxWidth)
	assert.Equal(t, high.CRF, highHDR.CRF, "High HDR encodes like High")
	assert.True(t, highHDR.KeepHDR, "High HDR keeps HDR for editing projects")
	assert.True(t, highHDR.QualityPreset().PreserveHDR, "and passes it on to its preset")
	assert.False(t, def.KeepHDR, "Medium converts HDR to SDR for social posts")
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
	require.NoError(t, db.DeleteClipProfile(t.Context(), "high-hdr"))

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
