// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionClock is a settable clock for a session store.
type sessionClock struct {
	// at is the time the clock reports.
	at time.Time
}

// read reports the clock's time.
//
// Returns:
//   - now: The current fake time.
func (clock *sessionClock) read() time.Time {
	return clock.at
}

// sessionStore opens a migrated database and a session store on a fake clock.
//
// Parameters:
//   - t: The test that owns the store.
//
// Returns:
//   - store: The session store under test.
//   - clock: The clock the store reads.
func sessionStore(t *testing.T) (*SessionStore, *sessionClock) {
	t.Helper()

	db, err := New(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	clock := &sessionClock{at: time.Unix(1_800_000_000, 0)}

	store := NewSessionStore(db)

	store.now = clock.read

	return store, clock
}

// storedSession reads a session row straight from the table.
//
// Parameters:
//   - t: The test reading the row.
//   - store: Store whose database is read.
//   - key: Session id.
//
// Returns:
//   - data: The stored bytes, nil when no row exists.
func storedSession(t *testing.T, store *SessionStore, key string) []byte {
	t.Helper()

	var data []byte

	err := store.db.conn.QueryRowContext(
		t.Context(),
		`SELECT data FROM sessions WHERE id = ?`,
		key,
	).Scan(&data)
	if err != nil {
		return nil
	}

	return data
}

func TestSessionStoreReportsAMissingSession(t *testing.T) {
	t.Parallel()

	store, _ := sessionStore(t)

	data, err := store.Get("absent")
	require.NoError(t, err)
	assert.Nil(t, data)

	data, err = store.Get("")
	require.NoError(t, err)
	assert.Nil(t, data)
}

func TestSessionStoreRoundTripsASession(t *testing.T) {
	t.Parallel()

	store, _ := sessionStore(t)

	require.NoError(t, store.Set("sid", []byte("payload"), time.Hour))

	data, err := store.Get("sid")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), data)
}

func TestSessionStoreHidesAnExpiredSession(t *testing.T) {
	t.Parallel()

	store, clock := sessionStore(t)

	require.NoError(t, store.Set("sid", []byte("payload"), time.Minute))

	clock.at = clock.at.Add(time.Minute)

	data, err := store.Get("sid")
	require.NoError(t, err)
	assert.Nil(t, data)
}

func TestSessionStoreKeepsASessionWithoutExpiry(t *testing.T) {
	t.Parallel()

	store, clock := sessionStore(t)

	require.NoError(t, store.Set("sid", []byte("payload"), 0))

	clock.at = clock.at.Add(365 * 24 * time.Hour)

	data, err := store.Get("sid")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), data)
}

func TestSessionStoreIgnoresAnEmptyKeyOrValue(t *testing.T) {
	t.Parallel()

	store, _ := sessionStore(t)

	require.NoError(t, store.Set("", []byte("payload"), time.Hour))
	require.NoError(t, store.Set("sid", nil, time.Hour))

	assert.Nil(t, storedSession(t, store, "sid"))
}

func TestSessionStoreReplacesASession(t *testing.T) {
	t.Parallel()

	store, _ := sessionStore(t)

	require.NoError(t, store.Set("sid", []byte("first"), time.Hour))
	require.NoError(t, store.Set("sid", []byte("second"), time.Hour))

	assert.Equal(t, []byte("second"), storedSession(t, store, "sid"))
}

func TestSessionStoreDeletesASession(t *testing.T) {
	t.Parallel()

	store, _ := sessionStore(t)

	require.NoError(t, store.Set("sid", []byte("payload"), time.Hour))
	require.NoError(t, store.Delete("sid"))

	assert.Nil(t, storedSession(t, store, "sid"))
}

func TestSessionStoreResetsEverySession(t *testing.T) {
	t.Parallel()

	store, _ := sessionStore(t)

	require.NoError(t, store.Set("one", []byte("payload"), time.Hour))
	require.NoError(t, store.Set("two", []byte("payload"), time.Hour))
	require.NoError(t, store.Reset())

	assert.Nil(t, storedSession(t, store, "one"))
	assert.Nil(t, storedSession(t, store, "two"))
	require.NoError(t, store.Close())
}

func TestSessionStoreSweepsOnlyExpiredSessions(t *testing.T) {
	t.Parallel()

	store, clock := sessionStore(t)

	require.NoError(t, store.Set("short", []byte("payload"), time.Minute))
	require.NoError(t, store.Set("long", []byte("payload"), time.Hour))
	require.NoError(t, store.Set("forever", []byte("payload"), 0))

	clock.at = clock.at.Add(2 * time.Minute)

	removed, err := store.Sweep(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)

	assert.Nil(t, storedSession(t, store, "short"))
	assert.NotNil(t, storedSession(t, store, "long"))
	assert.NotNil(t, storedSession(t, store, "forever"))
}

func TestSessionStoreReportsAClosedDatabase(t *testing.T) {
	t.Parallel()

	store, _ := sessionStore(t)
	require.NoError(t, store.db.Close())

	_, err := store.Get("sid")
	require.ErrorContains(t, err, "get session")

	err = store.Set("sid", []byte("payload"), time.Hour)
	require.ErrorContains(t, err, "set session")

	err = store.Delete("sid")
	require.ErrorContains(t, err, "delete session")

	err = store.Reset()
	require.ErrorContains(t, err, "reset sessions")

	_, err = store.Sweep(t.Context())
	require.ErrorContains(t, err, "sweep sessions")
}
