// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
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
	preset := profile.Preset(ctx, db, record.Quality)

	// The clip carries the Keep HDR it took from its profile when the render
	// was queued, which stays right if the profile changes or is deleted.
	preset.PreserveHDR = record.PreserveHDR

	return preset
}
