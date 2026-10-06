// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SessionStore keeps web sessions in the database. It satisfies the Fiber
// storage interface.
type SessionStore struct {
	db  *DB
	now func() time.Time
}

// NewSessionStore creates a session store backed by the database.
//
// Parameters:
//   - db: Database the sessions live in.
//
// Returns:
//   - store: A ready-to-use session store.
func NewSessionStore(db *DB) *SessionStore {
	return &SessionStore{
		db:  db,
		now: time.Now,
	}
}

// Close releases nothing, because the database belongs to its owner.
//
// Returns:
//   - err: Always nil.
func (*SessionStore) Close() error {
	return nil
}

// Delete removes a session.
//
// Parameters:
//   - key: Session id.
//
// Returns:
//   - err: Non-nil when the delete fails.
func (store *SessionStore) Delete(key string) error {
	//nolint:wrapcheck // The context variant wraps its own failure.
	return store.DeleteWithContext(context.Background(), key)
}

// DeleteWithContext removes a session.
//
// Parameters:
//   - ctx: Request scope for the delete.
//   - key: Session id.
//
// Returns:
//   - err: Non-nil when the delete fails.
func (store *SessionStore) DeleteWithContext(ctx context.Context, key string) error {
	_, err := store.db.conn.ExecContext(
		ctx,
		store.db.rewrite(`DELETE FROM sessions WHERE id = ?`),
		key,
	)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// Get loads a session.
//
// Parameters:
//   - key: Session id.
//
// Returns:
//   - data: The stored session, or nil when it is missing or expired.
//   - err: Non-nil when the read fails.
func (store *SessionStore) Get(key string) ([]byte, error) {
	//nolint:wrapcheck // The context variant wraps its own failure.
	return store.GetWithContext(context.Background(), key)
}

// GetWithContext loads a session.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - key: Session id.
//
// Returns:
//   - data: The stored session, or nil when it is missing or expired.
//   - err: Non-nil when the read fails.
func (store *SessionStore) GetWithContext(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, nil
	}

	var (
		data         []byte
		expiresAtSec int64
	)

	err := store.db.conn.QueryRowContext(
		ctx,
		store.db.rewrite(`SELECT data, expires_at FROM sessions WHERE id = ?`),
		key,
	).Scan(&data, &expiresAtSec)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get session: %w", err)
	}

	if expiresAtSec != 0 && expiresAtSec <= store.now().Unix() {
		return nil, nil
	}

	return data, nil
}

// Reset removes every session.
//
// Returns:
//   - err: Non-nil when the delete fails.
func (store *SessionStore) Reset() error {
	//nolint:wrapcheck // The context variant wraps its own failure.
	return store.ResetWithContext(context.Background())
}

// ResetWithContext removes every session.
//
// Parameters:
//   - ctx: Request scope for the delete.
//
// Returns:
//   - err: Non-nil when the delete fails.
func (store *SessionStore) ResetWithContext(ctx context.Context) error {
	_, err := store.db.conn.ExecContext(ctx, store.db.rewrite(`DELETE FROM sessions`))
	if err != nil {
		return fmt.Errorf("reset sessions: %w", err)
	}

	return nil
}

// Set stores a session.
//
// Parameters:
//   - key: Session id.
//   - val: Encoded session.
//   - exp: Lifetime of the session, zero for none.
//
// Returns:
//   - err: Non-nil when the write fails.
func (store *SessionStore) Set(key string, val []byte, exp time.Duration) error {
	//nolint:wrapcheck // The context variant wraps its own failure.
	return store.SetWithContext(context.Background(), key, val, exp)
}

// SetWithContext stores a session.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - key: Session id.
//   - val: Encoded session.
//   - exp: Lifetime of the session, zero for none.
//
// Returns:
//   - err: Non-nil when the write fails.
func (store *SessionStore) SetWithContext(
	ctx context.Context,
	key string,
	val []byte,
	exp time.Duration,
) error {
	if key == "" || len(val) == 0 {
		return nil
	}

	var expiresAtSec int64

	if exp > 0 {
		expiresAtSec = store.now().Add(exp).Unix()
	}

	_, err := store.db.conn.ExecContext(ctx, store.db.rewrite(`
		INSERT INTO sessions (id, data, expires_at)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			data = excluded.data,
			expires_at = excluded.expires_at
	`), key, val, expiresAtSec)
	if err != nil {
		return fmt.Errorf("set session: %w", err)
	}

	return nil
}

// Sweep deletes expired sessions.
//
// Parameters:
//   - ctx: Request scope for the delete.
//
// Returns:
//   - removed: How many sessions were deleted.
//   - err: Non-nil when the delete fails.
func (store *SessionStore) Sweep(ctx context.Context) (int64, error) {
	result, err := store.db.conn.ExecContext(
		ctx,
		store.db.rewrite(`DELETE FROM sessions WHERE expires_at != 0 AND expires_at <= ?`),
		store.now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("sweep sessions: %w", err)
	}

	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sweep sessions rows: %w", err)
	}

	return removed, nil
}
