// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Setting reads one value this installation persisted about itself.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - name: Setting to read.
//
// Returns:
//   - value: The stored value, or empty when none is stored.
//   - found: True when the setting is stored.
//   - err: Non-nil when the read fails.
func (db *DB) Setting(ctx context.Context, name string) (string, bool, error) {
	var value string

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT value FROM app_settings WHERE name = ?`),
		name,
	).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("setting %s: %w", name, err)
	}

	return value, true, nil
}

// SaveSetting upserts one value this installation persists about itself.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - name: Setting to write.
//   - value: Value to store.
//
// Returns:
//   - err: Non-nil when the row cannot be written.
func (db *DB) SaveSetting(ctx context.Context, name, value string) error {
	_, err := db.conn.ExecContext(ctx, db.rewrite(`
		INSERT INTO app_settings (name, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`), name, value)
	if err != nil {
		return fmt.Errorf("save setting %s: %w", name, err)
	}

	return nil
}
