// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// KeepHDR reports a profile's Keep HDR setting and whether the profile was
// found. A stored profile wins, then a built-in of the same id. Unlike Preset,
// it never answers with the Medium fallback's setting, so a caller can tell a
// profile that keeps HDR from one that is gone.
//
// Parameters:
//   - ctx: Database context.
//   - db: Clip profile store; may be nil.
//   - quality: The profile id, empty for the default profile.
//
// Returns:
//   - keep: Whether the profile keeps HDR.
//   - found: False when no stored or built-in profile has the id, or the
//     default profile cannot be read.
//
//nolint:nonamedreturns // Same-type returns need names.
func KeepHDR(ctx context.Context, db *database.DB, quality string) (keep, found bool) {
	if quality == "" {
		if db == nil {
			return clip.QualityPresets[clip.ClipQualityMedium].PreserveHDR, true
		}

		stored, err := db.DefaultClipProfile(ctx)
		if err != nil {
			return false, false
		}

		return stored.KeepHDR, true
	}

	if preset, ok := lookupPreset(ctx, db, quality); ok {
		return preset.PreserveHDR, true
	}

	if preset, ok := clip.QualityPresets[clip.ClipQuality(quality)]; ok {
		return preset.PreserveHDR, true
	}

	return false, false
}

// Preset resolves a stored quality id onto ffmpeg settings, including the
// profile's keep-HDR default.
//
// Parameters:
//   - ctx: Database context.
//   - db: Clip profile store; may be nil.
//   - quality: The stored quality id, empty for the default profile.
//
// Returns:
//   - preset: The resolved encode settings.
func Preset(ctx context.Context, db *database.DB, quality string) clip.QualityPreset {
	if quality == "" {
		return defaultPreset(ctx, db)
	}

	return clip.ResolvePreset(quality, func(id string) (clip.QualityPreset, bool) {
		return lookupPreset(ctx, db, id)
	})
}

// defaultPreset loads the stored default profile, or the built-in medium preset.
//
// Parameters:
//   - ctx: Database context.
//   - db: Clip profile store; may be nil.
//
// Returns:
//   - preset: The stored default profile's settings, or the built-in medium preset.
func defaultPreset(ctx context.Context, db *database.DB) clip.QualityPreset {
	if db == nil {
		return clip.QualityPresets[clip.ClipQualityMedium]
	}

	profile, err := db.DefaultClipProfile(ctx)
	if err != nil {
		return clip.QualityPresets[clip.ClipQualityMedium]
	}

	return clip.NormalizePreset(profile.QualityPreset())
}

// lookupPreset loads one stored profile by id.
//
// Parameters:
//   - ctx: Database context.
//   - db: Clip profile store; may be nil.
//   - id: The stored profile id.
//
// Returns:
//   - preset: The stored profile's settings, empty when it cannot be loaded.
//   - found: True when the profile was loaded.
func lookupPreset(ctx context.Context, db *database.DB, id string) (clip.QualityPreset, bool) {
	if db == nil {
		return clip.QualityPreset{
			CRF:         0,
			Preset:      "",
			AudioKbps:   0,
			MaxWidth:    0,
			PreserveHDR: false,
		}, false
	}

	profile, err := db.GetClipProfile(ctx, id)
	if err != nil {
		return clip.QualityPreset{
			CRF:         0,
			Preset:      "",
			AudioKbps:   0,
			MaxWidth:    0,
			PreserveHDR: false,
		}, false
	}

	return profile.QualityPreset(), true
}
