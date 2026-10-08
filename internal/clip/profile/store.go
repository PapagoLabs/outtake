// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/store/database"
)

// Profile is a validated clip encode profile, as the settings page edits it.
type Profile struct {
	ID        string
	Name      string
	CRF       int
	Preset    string
	AudioKbps int
	MaxWidth  int
	IsDefault bool
	// KeepHDR is the keep-HDR default for new video clips under this profile.
	KeepHDR   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProfileFields are the raw values a profile form posted.
type ProfileFields struct {
	// Name is the display name as typed.
	Name string
	// CRF is the constant rate factor as typed.
	CRF string
	// Preset is the libx264 encoder preset as typed.
	Preset string
	// AudioKbps is the audio bitrate as typed.
	AudioKbps string
	// MaxWidth is the maximum export width as typed.
	MaxWidth string
	// IsDefault reports whether the form asked for this profile to be default.
	IsDefault bool
	// KeepHDR reports whether new video clips from HDR sources keep HDR.
	KeepHDR bool
}

// Service stores and edits the clip profiles a user manages.
type Service struct {
	store *database.DB
}

// ProfileIDLength is the random clip profile identifier size in bytes.
const ProfileIDLength = 16

// New creates the clip profile service.
//
// Parameters:
//   - store: Persistence handle for the clip_profiles table.
//
// Returns:
//   - profiles: The clip profile service.
func New(store *database.DB) *Service {
	return &Service{store: store}
}

// Create validates the posted fields and stores a new profile.
//
// Parameters:
//   - ctx: Request context.
//   - fields: Raw form values.
//
// Returns:
//   - profile: The stored profile.
//   - err: A validation error naming the field that failed, or the write failure.
func (profiles *Service) Create(ctx context.Context, fields ProfileFields) (Profile, error) {
	profile, err := buildProfile(fields)
	if err != nil {
		return Profile{}, fmt.Errorf("create clip profile: %w", err)
	}

	err = profiles.store.SaveClipProfile(ctx, profile.record())
	if err != nil {
		return Profile{}, fmt.Errorf("create clip profile: %w", err)
	}

	return profile, nil
}

// buildProfile mints an identifier and validates the posted fields under it.
//
// Parameters:
//   - fields: Raw form values.
//
// Returns:
//   - profile: The validated profile, stamped with the current time.
//   - err: Wrapped error when no identifier could be minted or a field failed.
func buildProfile(fields ProfileFields) (Profile, error) {
	id, err := newProfileID()
	if err != nil {
		return Profile{}, fmt.Errorf("mint profile id: %w", err)
	}

	profile, err := ProfileFromFields(id, fields)
	if err != nil {
		return Profile{}, fmt.Errorf("validate profile: %w", err)
	}

	return profile, nil
}

// Delete removes a stored profile.
//
// Parameters:
//   - ctx: Request context.
//   - id: Profile to remove.
//
// Returns:
//   - err: Non-nil when it is the last profile or the row cannot be removed.
func (profiles *Service) Delete(ctx context.Context, id string) error {
	err := profiles.store.DeleteClipProfile(ctx, id)
	if err != nil {
		return fmt.Errorf("delete clip profile: %w", err)
	}

	return nil
}

// List returns the stored profiles, default first.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - profiles: The stored profiles, or nil when they cannot be read.
func (profiles *Service) List(ctx context.Context) []Profile {
	stored := StoredProfiles(ctx, profiles.store)
	items := make([]Profile, 0, len(stored))

	for i := range stored {
		items = append(items, profileFrom(stored[i]))
	}

	return items
}

// SetDefault marks a stored profile as the default.
//
// Parameters:
//   - ctx: Request context.
//   - id: Profile to mark.
//
// Returns:
//   - err: Non-nil when the profile is unknown or the flag cannot be assigned.
func (profiles *Service) SetDefault(ctx context.Context, id string) error {
	err := profiles.store.SetDefaultClipProfile(ctx, id)
	if err != nil {
		return fmt.Errorf("set default clip profile: %w", err)
	}

	return nil
}

// Update validates the posted fields and saves them over an existing profile.
//
// Parameters:
//   - ctx: Request context.
//   - id: Profile the edit applies to.
//   - fields: Raw form values.
//
// Returns:
//   - profile: The stored profile.
//   - err: A validation error naming the field that failed, the read failure, or
//     the write failure.
func (profiles *Service) Update(
	ctx context.Context,
	id string,
	fields ProfileFields,
) (Profile, error) {
	existing, err := profiles.store.GetClipProfile(ctx, id)
	if err != nil {
		return Profile{}, fmt.Errorf("load clip profile: %w", err)
	}

	profile, err := ProfileFromFields(id, fields)
	if err != nil {
		return Profile{}, fmt.Errorf("update clip profile: %w", err)
	}

	// An edit restates the encode settings, so the creation stamp and the
	// default flag are carried over rather than taken from the form.
	profile.CreatedAt = existing.CreatedAt
	profile.IsDefault = existing.IsDefault

	err = profiles.store.SaveClipProfile(ctx, profile.record())
	if err != nil {
		return Profile{}, fmt.Errorf("update clip profile: %w", err)
	}

	return profile, nil
}

// record maps a profile onto the persistence row it is stored as.
func (profile Profile) record() database.ClipProfile {
	return database.ClipProfile{
		ID:        profile.ID,
		Name:      profile.Name,
		CRF:       profile.CRF,
		Preset:    profile.Preset,
		AudioKbps: profile.AudioKbps,
		MaxWidth:  profile.MaxWidth,
		IsDefault: profile.IsDefault,
		KeepHDR:   profile.KeepHDR,
		CreatedAt: profile.CreatedAt,
		UpdatedAt: profile.UpdatedAt,
	}
}

// profileFrom maps a persistence row onto the profile the settings page edits.
//
// Parameters:
//   - record: Stored clip profile row.
//
// Returns:
//   - profile: The profile the settings page edits.
func profileFrom(record database.ClipProfile) Profile {
	return Profile{
		ID:        record.ID,
		Name:      record.Name,
		CRF:       record.CRF,
		Preset:    record.Preset,
		AudioKbps: record.AudioKbps,
		MaxWidth:  record.MaxWidth,
		IsDefault: record.IsDefault,
		KeepHDR:   record.KeepHDR,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

// newProfileID mints a random profile identifier.
//
// Returns:
//   - id: Hex-encoded random identifier.
//   - err: Non-nil when randomness is unavailable.
func newProfileID() (string, error) {
	var buf [ProfileIDLength]byte

	_, err := rand.Read(buf[:])
	if err != nil {
		return "", fmt.Errorf("generate profile id: %w", err)
	}

	return hex.EncodeToString(buf[:]), nil
}
