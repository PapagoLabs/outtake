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
	// KeepHDR is the keep-HDR default a new video clip takes from this profile.
	KeepHDR bool
}

// OutputWidth is a selectable clip export resolution with its UI label.
type OutputWidth struct {
	Width int
	Label string
}

// BuiltinProfiles returns the built-in clip profiles.
//
// Returns:
//   - options: The Low, Medium, and High built-ins, Medium marked default.
func BuiltinProfiles() []ProfileOption {
	return []ProfileOption{
		{ID: string(clip.ClipQualityLow), Name: "Low", IsDefault: false, KeepHDR: false},
		{ID: string(clip.ClipQualityMedium), Name: "Medium", IsDefault: true, KeepHDR: false},
		{ID: string(clip.ClipQualityHigh), Name: "High", IsDefault: false, KeepHDR: true},
	}
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
//     cannot be read, which is the caller's cue to fall back to the built-ins.
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
//   - options: The stored profiles, or the built-in profiles when none are
//     stored or the table cannot be read.
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
		return BuiltinProfiles()
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

// KeepsHDR reports the keep-HDR default a new video clip takes from a profile.
//
// Parameters:
//   - id: Selected profile id, empty for the default profile.
//   - options: Profiles a quality select offers.
//
// Returns:
//   - keep: The selected profile's default, or the default profile's when id
//     names none of the options, or false when no profile matches either.
func KeepsHDR(id string, options []ProfileOption) bool {
	for i := range options {
		if options[i].ID == id {
			return options[i].KeepHDR
		}
	}

	for i := range options {
		if options[i].IsDefault {
			return options[i].KeepHDR
		}
	}

	return false
}
