// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// UserRole returns the role of the user behind a Plex account.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - plexUserID: Plex account id to look up.
//
// Returns:
//   - role: The stored role, or empty when no user is stored.
//   - found: True when the account belongs to a user.
//   - err: Non-nil when the read fails.
func (db *DB) UserRole(ctx context.Context, plexUserID int) (string, bool, error) {
	var role string

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT role FROM users WHERE plex_user_id = ?`),
		plexUserID,
	).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("user role: %w", err)
	}

	return role, true, nil
}

// HasOwner reports whether a Plex account has claimed this installation.
//
// Parameters:
//   - ctx: Request scope for the read.
//
// Returns:
//   - owned: True when an owner is stored.
//   - err: Non-nil when the read fails.
func (db *DB) HasOwner(ctx context.Context) (bool, error) {
	var count int

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT COUNT(*) FROM users WHERE role = 'owner'`),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("has owner: %w", err)
	}

	return count > 0, nil
}

// ClaimOwner records a Plex account as the owner unless one is stored already.
//
// The unique owner index decides a race between two sign-ins, so exactly one
// of them reports the claim.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - plexUserID: Plex account id claiming the installation.
//   - username: Plex username of that account.
//
// Returns:
//   - claimed: True when this call stored the owner.
//   - err: Non-nil when the write fails.
func (db *DB) ClaimOwner(ctx context.Context, plexUserID int, username string) (bool, error) {
	result, err := db.conn.ExecContext(ctx, db.rewrite(`
		INSERT INTO users (plex_user_id, username, role, created_at, last_login_at)
		VALUES (?, ?, 'owner', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
	`), plexUserID, username)
	if err != nil {
		return false, fmt.Errorf("claim owner: %w", err)
	}

	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim owner rows: %w", err)
	}

	return inserted == 1, nil
}

// TouchLogin records a sign-in, refreshing the username Plex reported.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - plexUserID: Plex account id that signed in.
//   - username: Plex username of that account.
//
// Returns:
//   - err: Non-nil when the write fails.
func (db *DB) TouchLogin(ctx context.Context, plexUserID int, username string) error {
	_, err := db.conn.ExecContext(ctx, db.rewrite(`
		UPDATE users SET username = ?, last_login_at = CURRENT_TIMESTAMP
		WHERE plex_user_id = ?
	`), username, plexUserID)
	if err != nil {
		return fmt.Errorf("touch login: %w", err)
	}

	return nil
}

// ResetOwner deletes the owner, the selected server, and the rows in
// plex_tokens, so the next Plex account to sign in claims the installation and
// picks its own server.
//
// Parameters:
//   - ctx: Request scope for the transaction.
//
// Returns:
//   - removed: True when an owner was stored.
//   - err: Non-nil when a delete or the commit fails.
func (db *DB) ResetOwner(ctx context.Context) (bool, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin reset owner: %w", err)
	}

	defer func() {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return
		}
	}()

	removed, err := db.deleteOwnerRows(ctx, tx)
	if err != nil {
		return false, fmt.Errorf("reset owner: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return false, fmt.Errorf("commit reset owner: %w", err)
	}

	return removed, nil
}

// deleteOwnerRows deletes the owner, the selected server, and the legacy
// tokens inside a transaction.
//
// Parameters:
//   - ctx: Request scope for the deletes.
//   - tx: Transaction the deletes run in.
//
// Returns:
//   - removed: True when an owner was stored.
//   - err: Non-nil when a delete fails.
func (db *DB) deleteOwnerRows(ctx context.Context, tx *sql.Tx) (bool, error) {
	result, err := tx.ExecContext(ctx, db.rewrite(`DELETE FROM users WHERE role = 'owner'`))
	if err != nil {
		return false, fmt.Errorf("delete owner: %w", err)
	}

	removed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete owner rows: %w", err)
	}

	_, err = tx.ExecContext(ctx, db.rewrite(`DELETE FROM selected_server`))
	if err != nil {
		return false, fmt.Errorf("clear selected server: %w", err)
	}

	_, err = tx.ExecContext(ctx, db.rewrite(`DELETE FROM plex_tokens`))
	if err != nil {
		return false, fmt.Errorf("clear legacy tokens: %w", err)
	}

	return removed > 0, nil
}
