// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/media"
)

// ClipProfile is a user-managed clip encode profile.
type ClipProfile struct {
	ID        string
	Name      string
	CRF       int
	Preset    string
	AudioKbps int
	MaxWidth  int
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

const (
	// ClipProfileSelectCols is the clip_profiles projection used by read queries.
	clipProfileSelectCols = `id, name, crf, preset, audio_kbps, max_width, is_default, created_at, updated_at`
)

// ErrClipProfileNotFound is returned when a clip profile row does not exist.
var ErrClipProfileNotFound = errors.New("clip profile not found")

// ErrLastClipProfile is returned when deleting the only remaining profile.
var ErrLastClipProfile = errors.New("cannot delete the last clip profile")

// SaveClipProfile inserts or replaces a clip profile.
func (db *DB) SaveClipProfile(ctx context.Context, profile ClipProfile) error {
	isDefault := 0
	if profile.IsDefault {
		isDefault = 1
	}

	_, err := db.conn.ExecContext(ctx, db.rewrite(`
		INSERT INTO clip_profiles (
			id, name, crf, preset, audio_kbps, max_width, is_default, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			crf = excluded.crf,
			preset = excluded.preset,
			audio_kbps = excluded.audio_kbps,
			max_width = excluded.max_width,
			is_default = excluded.is_default,
			updated_at = excluded.updated_at
	`),
		profile.ID,
		profile.Name,
		profile.CRF,
		profile.Preset,
		profile.AudioKbps,
		profile.MaxWidth,
		isDefault,
		profile.CreatedAt,
		profile.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save clip profile: %w", err)
	}

	if profile.IsDefault {
		err = db.assignDefaultClipProfile(ctx, profile.ID)
		if err != nil {
			return fmt.Errorf("save clip profile: %w", err)
		}

		return nil
	}

	err = db.ensureDefaultClipProfile(ctx)
	if err != nil {
		return fmt.Errorf("ensure default after save: %w", err)
	}

	return nil
}

// GetClipProfile loads a clip profile by ID.
func (db *DB) GetClipProfile(ctx context.Context, id string) (ClipProfile, error) {
	row := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT `+clipProfileSelectCols+` FROM clip_profiles WHERE id = ?`),
		id,
	)

	profile, err := scanClipProfile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ClipProfile{}, ErrClipProfileNotFound
		}

		return ClipProfile{}, fmt.Errorf("get clip profile: %w", err)
	}

	return profile, nil
}

// ListClipProfiles returns clip profiles with the default first.
func (db *DB) ListClipProfiles(ctx context.Context) ([]ClipProfile, error) {
	rows, err := db.conn.QueryContext(
		ctx,
		db.rewrite(`SELECT `+clipProfileSelectCols+` FROM clip_profiles
			ORDER BY is_default DESC, `+db.nameOrder()),
	)
	if err != nil {
		return nil, fmt.Errorf("list clip profiles: %w", err)
	}
	defer rows.Close()

	profiles := make([]ClipProfile, 0)

	for rows.Next() {
		profile, scanErr := scanClipProfile(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan clip profiles: %w", scanErr)
		}

		profiles = append(profiles, profile)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate clip profiles: %w", err)
	}

	return profiles, nil
}

// DefaultClipProfile returns the profile marked default, or Medium built-in.
func (db *DB) DefaultClipProfile(ctx context.Context) (ClipProfile, error) {
	row := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT `+clipProfileSelectCols+` FROM clip_profiles
			WHERE is_default = 1 LIMIT 1`),
	)

	profile, err := scanClipProfile(row)
	if err == nil {
		return profile, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return ClipProfile{}, fmt.Errorf("default clip profile: %w", err)
	}

	fallback, getErr := db.GetClipProfile(ctx, string(media.ClipQualityMedium))
	if getErr != nil {
		return ClipProfile{}, fmt.Errorf("default clip profile: %w", getErr)
	}

	return fallback, nil
}

// SetDefaultClipProfile marks one profile as the default.
func (db *DB) SetDefaultClipProfile(ctx context.Context, id string) error {
	_, err := db.GetClipProfile(ctx, id)
	if err != nil {
		return fmt.Errorf("set default clip profile: %w", err)
	}

	err = db.assignDefaultClipProfile(ctx, id)
	if err != nil {
		return fmt.Errorf("set default clip profile: %w", err)
	}

	return nil
}

// DeleteClipProfile removes a profile and keeps a default assigned.
func (db *DB) DeleteClipProfile(ctx context.Context, id string) error {
	count, err := db.clipProfileCount(ctx)
	if err != nil {
		return fmt.Errorf("delete clip profile: %w", err)
	}

	if count <= 1 {
		return ErrLastClipProfile
	}

	_, err = db.GetClipProfile(ctx, id)
	if err != nil {
		return fmt.Errorf("lookup clip profile: %w", err)
	}

	_, err = db.conn.ExecContext(ctx, db.rewrite(`DELETE FROM clip_profiles WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("exec delete clip profile: %w", err)
	}

	err = db.ensureDefaultClipProfile(ctx)
	if err != nil {
		return fmt.Errorf("delete clip profile: %w", err)
	}

	return nil
}

// QualityPreset returns ffmpeg settings for a stored profile.
func (profile ClipProfile) QualityPreset() media.QualityPreset {
	return media.QualityPreset{
		CRF:       profile.CRF,
		Preset:    profile.Preset,
		AudioKbps: profile.AudioKbps,
		MaxWidth:  profile.MaxWidth,
	}
}

// clipProfileCount returns the number of stored profiles.
func (db *DB) clipProfileCount(ctx context.Context) (int, error) {
	var count int

	err := db.conn.QueryRowContext(ctx, db.rewrite(`SELECT COUNT(*) FROM clip_profiles`)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count clip profiles: %w", err)
	}

	return count, nil
}

// assignDefaultClipProfile sets one default in a single UPDATE.
func (db *DB) assignDefaultClipProfile(ctx context.Context, id string) error {
	_, err := db.conn.ExecContext(
		ctx,
		db.rewrite(`UPDATE clip_profiles
			SET is_default = CASE WHEN id = ? THEN 1 ELSE 0 END,
			    updated_at = CURRENT_TIMESTAMP`),
		id,
	)
	if err != nil {
		return fmt.Errorf("assign default clip profile: %w", err)
	}

	return nil
}

// ensureDefaultClipProfile assigns a default when none is set.
func (db *DB) ensureDefaultClipProfile(ctx context.Context) error {
	var defaults int

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT COUNT(*) FROM clip_profiles WHERE is_default = 1`),
	).Scan(&defaults)
	if err != nil {
		return fmt.Errorf("count default clip profiles: %w", err)
	}

	if defaults > 0 {
		return nil
	}

	_, err = db.conn.ExecContext(
		ctx,
		db.rewrite(`UPDATE clip_profiles SET is_default = 1
			WHERE id = (
				SELECT id FROM clip_profiles ORDER BY `+db.nameOrder()+` LIMIT 1
			)`),
	)
	if err != nil {
		return fmt.Errorf("ensure default clip profile: %w", err)
	}

	return nil
}

// scanClipProfile reads one clip profile row.
func scanClipProfile(row scannable) (ClipProfile, error) {
	var profile ClipProfile
	var isDefault int

	err := row.Scan(
		&profile.ID,
		&profile.Name,
		&profile.CRF,
		&profile.Preset,
		&profile.AudioKbps,
		&profile.MaxWidth,
		&isDefault,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if err != nil {
		return ClipProfile{}, fmt.Errorf("scan clip profile: %w", err)
	}

	profile.IsDefault = isDefault != 0

	return profile, nil
}
