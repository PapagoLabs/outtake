// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip/quota"
)

// UserRecord is one user as the users page lists them.
type UserRecord struct {
	// CreatedAt is when the account first signed in.
	CreatedAt time.Time
	// LastLoginAt is when it last signed in, zero when it never did.
	LastLoginAt time.Time
	// Limits are the user's own render limits.
	Limits quota.Override
	// Username is the Plex username.
	Username string
	// Role is owner, admin, member, or pending.
	Role string
	// PlexID is the plex.tv account id.
	PlexID int
}

// Roles a user is stored or reported with.
const (
	// RoleOwner is the installation's owner.
	RoleOwner = "owner"
	// RoleAdmin manages the installation on the owner's behalf.
	RoleAdmin = "admin"
	// RoleMember uses the installation.
	RoleMember = "member"
	// RolePending is reported for an account an admin has not approved.
	RolePending = "pending"
)

// ErrUserNotFound is returned for a change to a user that is not stored, or
// to the owner, whom no change on the users page touches.
var ErrUserNotFound = errors.New("user not found")

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
	var (
		role     string
		approved int
	)

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT role, approved FROM users WHERE plex_user_id = ?`),
		plexUserID,
	).Scan(&role, &approved)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("user role: %w", err)
	}

	if approved == 0 {
		return RolePending, true, nil
	}

	return role, true, nil
}

// AddPendingUser records a Plex account that waits for an admin's approval.
//
// An account already stored keeps its row.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - plexUserID: Plex account id asking to use the installation.
//   - username: Plex username of that account.
//
// Returns:
//   - err: Non-nil when the write fails.
func (db *DB) AddPendingUser(ctx context.Context, plexUserID int, username string) error {
	_, err := db.conn.ExecContext(ctx, db.rewrite(`
		INSERT INTO users (plex_user_id, username, role, approved, created_at, last_login_at)
		VALUES (?, ?, 'member', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
	`), plexUserID, username)
	if err != nil {
		return fmt.Errorf("add pending user: %w", err)
	}

	return nil
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
// of them reports the claim. The claim also takes over every clip that belongs
// to nobody, such as the clips of an installation upgraded from a release
// without owners, in the same transaction.
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
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin claim owner: %w", err)
	}

	defer func() {
		//nolint:errcheck,revive // After a commit it answers sql.ErrTxDone, with nothing to undo.
		tx.Rollback()
	}()

	claimed, err := db.claimOwnerRows(ctx, tx, plexUserID, username)
	if err != nil {
		return false, fmt.Errorf("claim: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return false, fmt.Errorf("commit claim owner: %w", err)
	}

	return claimed, nil
}

// claimOwnerRows stores the owner and hands it every clip that belongs to
// nobody, inside a transaction.
//
// Parameters:
//   - ctx: Request scope for the writes.
//   - tx: Transaction the writes run in.
//   - plexUserID: Plex account id claiming the installation.
//   - username: Plex username of that account.
//
// Returns:
//   - claimed: True when the owner was stored.
//   - err: Non-nil when a write fails.
func (db *DB) claimOwnerRows(
	ctx context.Context,
	tx *sql.Tx,
	plexUserID int,
	username string,
) (bool, error) {
	result, err := tx.ExecContext(ctx, db.rewrite(`
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

	if inserted != 1 {
		return false, nil
	}

	_, err = tx.ExecContext(
		ctx,
		db.rewrite(`UPDATE clips SET owner_id = ? WHERE owner_id = 0`),
		plexUserID,
	)
	if err != nil {
		return false, fmt.Errorf("give ownerless clips to the owner: %w", err)
	}

	return true, nil
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

// ResetOwner deletes every user, the selected server, and the rows in
// plex_tokens, so the next Plex account to sign in claims the installation and
// picks its own server.
//
// The members and admins go too, because they were admitted to the old
// owner's server.
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

// deleteOwnerRows deletes every user, the selected server, and the legacy
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
	var owners int

	err := tx.QueryRowContext(ctx, db.rewrite(`SELECT COUNT(*) FROM users WHERE role = 'owner'`)).
		Scan(&owners)
	if err != nil {
		return false, fmt.Errorf("count owner: %w", err)
	}

	_, err = tx.ExecContext(ctx, db.rewrite(`DELETE FROM users`))
	if err != nil {
		return false, fmt.Errorf("delete users: %w", err)
	}

	_, err = tx.ExecContext(ctx, db.rewrite(`DELETE FROM selected_server`))
	if err != nil {
		return false, fmt.Errorf("clear selected server: %w", err)
	}

	_, err = tx.ExecContext(ctx, db.rewrite(`DELETE FROM plex_tokens`))
	if err != nil {
		return false, fmt.Errorf("clear legacy tokens: %w", err)
	}

	return owners > 0, nil
}

// ApproveUser lets a pending account use the installation as a member.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - plexUserID: The pending account.
//
// Returns:
//   - err: ErrUserNotFound when no pending account is stored under the id,
//     otherwise the write failure.
func (db *DB) ApproveUser(ctx context.Context, plexUserID int) error {
	err := db.changeUser(
		ctx,
		`UPDATE users SET approved = 1 WHERE plex_user_id = ? AND approved = 0`,
		plexUserID,
	)
	if err != nil {
		return fmt.Errorf("approve user: %w", err)
	}

	return nil
}

// ListUsers returns every stored user, the owner first, then by when each
// first signed in.
//
// Parameters:
//   - ctx: Request scope for the read.
//
// Returns:
//   - users: Every user.
//   - err: Non-nil when the read fails.
func (db *DB) ListUsers(ctx context.Context) ([]UserRecord, error) {
	rows, err := db.conn.QueryContext(ctx, db.rewrite(`
		SELECT plex_user_id, username, role, approved, created_at, last_login_at,
			limit_active, limit_weekly, limit_storage_bytes, limit_clip_seconds
		FROM users
		ORDER BY CASE WHEN role = 'owner' THEN 0 ELSE 1 END, created_at, id
	`))
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	defer func() { _ = rows.Close() }()

	users := make([]UserRecord, 0)

	for rows.Next() {
		user, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan users: %w", scanErr)
		}

		users = append(users, user)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("read users: %w", err)
	}

	return users, nil
}

// RejectUser deletes an account still waiting for approval.
//
// An account approved in the meantime is left alone.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - plexUserID: The pending account.
//
// Returns:
//   - err: ErrUserNotFound when no pending account is stored under the id,
//     otherwise the write failure.
func (db *DB) RejectUser(ctx context.Context, plexUserID int) error {
	err := db.changeUser(
		ctx,
		`DELETE FROM users WHERE plex_user_id = ? AND approved = 0 AND role <> 'owner'`,
		plexUserID,
	)
	if err != nil {
		return fmt.Errorf("reject user: %w", err)
	}

	return nil
}

// RemoveUser deletes a user other than the owner.
//
// The user's sessions end on their next request, and their clips are kept.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - plexUserID: The user.
//
// Returns:
//   - err: ErrUserNotFound when no user other than the owner is stored under
//     the id, otherwise the write failure.
func (db *DB) RemoveUser(ctx context.Context, plexUserID int) error {
	err := db.changeUser(
		ctx,
		`DELETE FROM users WHERE plex_user_id = ? AND role <> 'owner'`,
		plexUserID,
	)
	if err != nil {
		return fmt.Errorf("remove user: %w", err)
	}

	return nil
}

// SetUserRole makes an approved member an admin, or an admin a member.
//
// The user must still hold the other role when the row is written, so a
// change checked against one role never lands on a user whose role has
// changed since.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - plexUserID: The user.
//   - role: RoleAdmin or RoleMember.
//
// Returns:
//   - err: ErrUserNotFound when no approved user holding the other role is
//     stored under the id, otherwise the write failure.
func (db *DB) SetUserRole(ctx context.Context, plexUserID int, role string) error {
	current := RoleMember

	switch role {
	case RoleAdmin:
	case RoleMember:
		current = RoleAdmin
	default:
		return fmt.Errorf("set user role %q: %w", role, ErrUserNotFound)
	}

	err := db.changeUser(
		ctx,
		`UPDATE users SET role = ? WHERE plex_user_id = ? AND role = ? AND approved = 1`,
		role,
		plexUserID,
		current,
	)
	if err != nil {
		return fmt.Errorf("set user role: %w", err)
	}

	return nil
}

// Usernames returns every approved user's Plex username by plex.tv id.
//
// An account still waiting for approval is left out, so it neither turns on
// sharing nor names a removed member who asked again.
//
// Parameters:
//   - ctx: Request scope for the read.
//
// Returns:
//   - names: Each approved user's username.
//   - err: Non-nil when the read fails.
func (db *DB) Usernames(ctx context.Context) (map[int]string, error) {
	rows, err := db.conn.QueryContext(ctx, db.rewrite(
		`SELECT plex_user_id, username FROM users WHERE approved = 1`,
	))
	if err != nil {
		return nil, fmt.Errorf("usernames: %w", err)
	}

	defer func() { _ = rows.Close() }()

	names := make(map[int]string)

	for rows.Next() {
		var (
			id   int
			name string
		)

		err = rows.Scan(&id, &name)
		if err != nil {
			return nil, fmt.Errorf("scan usernames: %w", err)
		}

		names[id] = name
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("read usernames: %w", err)
	}

	return names, nil
}

// changeUser runs one change to a user and reports a change that matched no
// row.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - query: The change, with ? placeholders.
//   - args: The placeholders' values.
//
// Returns:
//   - err: ErrUserNotFound when no row matched, otherwise the write failure.
func (db *DB) changeUser(ctx context.Context, query string, args ...any) error {
	result, err := db.conn.ExecContext(ctx, db.rewrite(query), args...)
	if err != nil {
		return fmt.Errorf("change user: %w", err)
	}

	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("change user rows: %w", err)
	}

	if changed == 0 {
		return ErrUserNotFound
	}

	return nil
}

// scanUser reads one user row.
//
// Parameters:
//   - row: A row matching ListUsers's columns.
//
// Returns:
//   - user: The user.
//   - err: Non-nil when a column cannot be scanned.
func scanUser(row scannable) (UserRecord, error) {
	var (
		user     UserRecord
		approved int
		created  sql.NullTime
		login    sql.NullTime
		limits   overrideColumns
	)

	err := row.Scan(
		&user.PlexID, &user.Username, &user.Role, &approved, &created, &login,
		&limits.active, &limits.weekly, &limits.storage, &limits.clipSeconds,
	)
	if err != nil {
		return UserRecord{}, fmt.Errorf("scan user: %w", err)
	}

	if approved == 0 {
		user.Role = RolePending
	}

	user.CreatedAt = created.Time
	user.LastLoginAt = login.Time
	user.Limits = limits.override()

	return user, nil
}
