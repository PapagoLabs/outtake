// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/PapagoLabs/outtake/internal/store/database"
)

// ErrUnknownProfile reports a quality that names no stored profile.
var ErrUnknownProfile = errors.New("unknown clip profile")

// ResolveProfile maps an empty or named quality onto a profile a clip can be
// encoded with.
//
// Parameters:
//   - ctx: Request context.
//   - store: Persistence handle for the clip_profiles table.
//   - quality: Requested profile id, empty for the default profile.
//
// Returns:
//   - id: The profile a clip should carry.
//   - err: ErrUnknownProfile when no stored profile has the id, or the
//     underlying failure when the profiles cannot be read.
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

	return "", ErrUnknownProfile
}
