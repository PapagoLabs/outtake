// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// clipEncodePreset resolves quality settings and the clip's color flags.
//
// Parameters:
//   - ctx: Database context.
//   - db: Clip profile store; may be nil.
//   - record: Clip whose quality and color flags are applied.
//
// Returns:
//   - preset: Encode settings with the color decisions applied.
func clipEncodePreset(
	ctx context.Context,
	db *database.DB,
	record *clip.Clip,
) clip.QualityPreset {
	preset := clipPreset(ctx, db, record.Quality)

	preset.WebSafeColor = record.WebSafeColor
	// The clip decides; the server setting only seeds the form's default state.
	preset.PreserveHDR = record.PreserveHDR

	return preset
}

// clipPreset resolves a stored quality id onto ffmpeg settings.
//
// Parameters:
//   - ctx: Database context.
//   - db: Clip profile store; may be nil.
//   - quality: The stored quality id, empty for the default profile.
//
// Returns:
//   - preset: The resolved encode settings.
func clipPreset(ctx context.Context, db *database.DB, quality string) clip.QualityPreset {
	if quality == "" {
		return defaultClipPreset(ctx, db)
	}

	return clip.ResolvePreset(quality, func(id string) (clip.QualityPreset, bool) {
		return lookupClipPreset(ctx, db, id)
	})
}

// defaultClipPreset loads the stored default profile, or the built-in medium preset.
//
// Parameters:
//   - ctx: Database context.
//   - db: Clip profile store; may be nil.
//
// Returns:
//   - preset: The stored default profile's settings, or the built-in medium preset.
func defaultClipPreset(ctx context.Context, db *database.DB) clip.QualityPreset {
	if db == nil {
		return clip.QualityPresets[clip.ClipQualityMedium]
	}

	profile, err := db.DefaultClipProfile(ctx)
	if err != nil {
		return clip.QualityPresets[clip.ClipQualityMedium]
	}

	return clip.NormalizePreset(profile.QualityPreset())
}

// lookupClipPreset loads one stored profile by id.
//
// Parameters:
//   - ctx: Database context.
//   - db: Clip profile store; may be nil.
//   - id: The stored profile id.
//
// Returns:
//   - preset: The stored profile's settings, empty when it cannot be loaded.
//   - found: True when the profile was loaded.
func lookupClipPreset(ctx context.Context, db *database.DB, id string) (clip.QualityPreset, bool) {
	if db == nil {
		return clip.QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}, false
	}

	profile, err := db.GetClipProfile(ctx, id)
	if err != nil {
		return clip.QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}, false
	}

	return profile.QualityPreset(), true
}
