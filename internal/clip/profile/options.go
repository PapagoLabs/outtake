// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// ProfileOption is a clip encode profile as a quality select offers it.
type ProfileOption struct {
	ID        string
	Name      string
	IsDefault bool
	// KeepHDR reports whether a video clip rendered with this profile keeps HDR.
	KeepHDR bool
}

// OutputWidth is a selectable clip export resolution with its UI label.
type OutputWidth struct {
	Width int
	Label string
}

// FallbackProfiles returns what a quality select offers when no stored profile
// can be read.
//
// Returns:
//   - options: One option with an empty id, which every caller resolves to
//     the default profile, named after [clip.DefaultPreset].
func FallbackProfiles() []ProfileOption {
	return []ProfileOption{{
		ID:        "",
		Name:      clip.OutputWidthLabel(clip.DefaultPreset.MaxWidth),
		IsDefault: true,
		KeepHDR:   clip.DefaultPreset.PreserveHDR,
	}}
}

// EncoderPresets lists the -preset values libx264 and libx265 share, from
// fastest to slowest.
//
// Returns:
//   - presets: A copy of the supported encoder presets.
func EncoderPresets() []string {
	presets := make([]string, 0, len(clip.EncoderPresets))

	presets = append(presets, clip.EncoderPresets...)

	return presets
}

// OutputWidths lists selectable clip export resolutions with their labels.
//
// Returns:
//   - widths: Every supported export resolution, narrowest first.
func OutputWidths() []OutputWidth {
	widths := make([]OutputWidth, 0, len(clip.OutputWidths))

	for _, width := range clip.OutputWidths {
		widths = append(widths, OutputWidth{
			Width: width,
			Label: clip.OutputWidthLabel(width),
		})
	}

	return widths
}

// StoredProfiles returns the persisted clip encode profiles.
//
// Parameters:
//   - ctx: Request context.
//   - store: Persistence handle for the clip_profiles table.
//
// Returns:
//   - profiles: Stored profiles with the default first, or nil when the table
//     cannot be read, which is the caller's cue to fall back to
//     FallbackProfiles.
func StoredProfiles(ctx context.Context, store *database.DB) []database.ClipProfile {
	profiles, err := store.ListClipProfiles(ctx)
	if err != nil {
		return nil
	}

	return profiles
}

// SelectableProfiles returns the profiles a quality select offers.
//
// Parameters:
//   - ctx: Request context.
//   - store: Persistence handle for the clip_profiles table.
//
// Returns:
//   - options: The stored profiles, or FallbackProfiles when none are stored
//     or the table cannot be read.
func SelectableProfiles(ctx context.Context, store *database.DB) []ProfileOption {
	stored := StoredProfiles(ctx, store)
	options := make([]ProfileOption, 0, len(stored))

	for i := range stored {
		record := stored[i]

		options = append(options, ProfileOption{
			ID:        record.ID,
			Name:      record.Name,
			IsDefault: record.IsDefault,
			KeepHDR:   record.KeepHDR,
		})
	}

	if len(options) == 0 {
		return FallbackProfiles()
	}

	return options
}

// ProfileName returns the name a profile id is shown under, falling back to the
// id itself when no offered profile carries it.
//
// Parameters:
//   - id: Profile identifier recorded on a clip.
//   - options: Profiles a quality select offers.
//
// Returns:
//   - name: The profile name, or the id when it is not one of the offered profiles.
func ProfileName(id string, options []ProfileOption) string {
	for i := range options {
		if options[i].ID == id {
			return options[i].Name
		}
	}

	return id
}
