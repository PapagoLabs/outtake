// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// ErrUnknownProfile reports a quality that names neither a stored profile nor a
// built-in preset.
var ErrUnknownProfile = errors.New("unknown clip profile")

// ResolveProfile maps an empty or named quality onto a profile a clip can be
// encoded with.
//
// Parameters:
//   - ctx: Request context.
//   - store: Persistence handle for the clip_profiles table.
//   - quality: Requested profile id or built-in quality name.
//
// Returns:
//   - id: The profile a clip should carry.
//   - err: ErrUnknownProfile when the name is neither stored nor built in, or
//     the underlying failure when the profiles cannot be read.
func ResolveProfile(ctx context.Context, store *database.DB, quality string) (string, error) {
	if quality == "" {
		profile, err := store.DefaultClipProfile(ctx)
		if err != nil {
			return "", fmt.Errorf("resolve profile: %w", err)
		}

		return profile.ID, nil
	}

	_, err := store.GetClipProfile(ctx, quality)
	if err == nil {
		return quality, nil
	}

	if !errors.Is(err, database.ErrClipProfileNotFound) {
		return "", fmt.Errorf("resolve profile: %w", err)
	}

	if _, ok := clip.QualityPresets[clip.ClipQuality(quality)]; ok {
		return quality, nil
	}

	return "", ErrUnknownProfile
}
