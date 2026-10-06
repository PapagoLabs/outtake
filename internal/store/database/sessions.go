// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

// SessionStore keeps web sessions in the database. It satisfies the Fiber
// storage interface.
//
// The record of recent writes lives in this process, so the store assumes one
// running instance per database.
type SessionStore struct {
	db  *DB
	now func() time.Time

	mu      sync.Mutex
	written map[string]sessionWrite
}

// sessionWrite is the last value written for a session and when.
type sessionWrite struct {
	// data is the stored value.
	data []byte
	// at is when it was written.
	at time.Time
}

// sessionWriteInterval is how long an unchanged session goes without being
// written again. A session is saved on every request, so without this every
// page load would be a database write. It is far shorter than the session idle
// timeout, so a skipped write never lets a live session expire.
const sessionWriteInterval = time.Minute

// NewSessionStore creates a session store backed by the database.
//
// Parameters:
//   - db: Database the sessions live in.
//
// Returns:
//   - store: A ready-to-use session store.
func NewSessionStore(db *DB) *SessionStore {
	return &SessionStore{
		db:      db,
		now:     time.Now,
		mu:      sync.Mutex{},
		written: make(map[string]sessionWrite),
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
	store.forget(key)

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
	store.mu.Lock()
	clear(store.written)
	store.mu.Unlock()

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

// SetWithContext stores a session. An unchanged session written within the
// last minute is not written again.
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
	if key == "" || len(val) == 0 || store.unchanged(key, val) {
		return nil
	}

	now := store.now()

	var expiresAtSec int64

	if exp > 0 {
		expiresAtSec = now.Add(exp).Unix()
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

	store.mu.Lock()

	store.written[key] = sessionWrite{data: bytes.Clone(val), at: now}

	store.mu.Unlock()

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
	now := store.now()

	store.mu.Lock()

	for key, write := range store.written {
		if now.Sub(write.at) >= sessionWriteInterval {
			delete(store.written, key)
		}
	}
	store.mu.Unlock()

	result, err := store.db.conn.ExecContext(
		ctx,
		store.db.rewrite(`DELETE FROM sessions WHERE expires_at != 0 AND expires_at <= ?`),
		now.Unix(),
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

// forget drops the record of a session's last write.
//
// Parameters:
//   - key: Session id.
func (store *SessionStore) forget(key string) {
	store.mu.Lock()
	delete(store.written, key)
	store.mu.Unlock()
}

// unchanged reports whether a session was written with the same value within
// the write interval.
//
// Parameters:
//   - key: Session id.
//   - val: Encoded session about to be written.
//
// Returns:
//   - skip: True when the write can be skipped.
func (store *SessionStore) unchanged(key string, val []byte) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	write, ok := store.written[key]

	return ok && bytes.Equal(write.data, val) && store.now().Sub(write.at) < sessionWriteInterval
}
