// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

func TestNew(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/test.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	err = db.Conn().PingContext(t.Context())
	require.NoError(t, err)
}

// TestSQLiteConnectionsUseWALAndWaitForLocks covers the settings every pooled
// connection is opened with, read back through several connections at once.
func TestSQLiteConnectionsUseWALAndWaitForLocks(t *testing.T) {
	t.Parallel()

	db, err := New(filepath.Join(t.TempDir(), "pragma.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	conns := make([]*sql.Conn, 0, sqliteConns)

	for range sqliteConns {
		conn, connErr := db.conn.Conn(t.Context())
		require.NoError(t, connErr)

		conns = append(conns, conn)
	}

	for _, conn := range conns {
		var mode string

		require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode))
		assert.Equal(t, "wal", mode)

		var timeout int

		require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&timeout))
		assert.Equal(t, 5000, timeout)

		var synchronous int

		require.NoError(
			t,
			conn.QueryRowContext(t.Context(), "PRAGMA synchronous").Scan(&synchronous),
		)
		assert.Equal(t, 1, synchronous, "NORMAL")

		require.NoError(t, conn.Close())
	}
}

// TestSQLiteTransactionsTakeTheWriteLockUpFront covers the immediate
// transaction mode: a second transaction waits for the first to finish rather
// than starting at once and failing when it later tries to write.
func TestSQLiteTransactionsTakeTheWriteLockUpFront(t *testing.T) {
	t.Parallel()

	db, err := New(filepath.Join(t.TempDir(), "txlock.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	first, err := db.conn.BeginTx(t.Context(), nil)
	require.NoError(t, err)

	began := make(chan error, 1)

	go func() {
		second, beginErr := db.conn.BeginTx(t.Context(), nil)
		if beginErr == nil {
			beginErr = second.Rollback()
		}

		began <- beginErr
	}()

	select {
	case <-began:
		t.Fatal("a second transaction began while the first held the write lock")
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, first.Commit())

	select {
	case beginErr := <-began:
		require.NoError(t, beginErr, "the second transaction begins once the lock is free")
	case <-time.After(3 * time.Second):
		t.Fatal("the second transaction never began")
	}
}

// TestSQLiteMemoryDatabaseKeepsOneConnection covers an in-memory database,
// where each connection would otherwise open a database of its own.
func TestSQLiteMemoryDatabaseKeepsOneConnection(t *testing.T) {
	t.Parallel()

	db, err := New(":memory:")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	assert.Equal(t, 1, db.conn.Stats().MaxOpenConnections)

	var tables int

	require.NoError(t, db.conn.QueryRowContext(t.Context(),
		`SELECT count(*) FROM sqlite_master WHERE name = 'schema_migrations'`).Scan(&tables))
	assert.Equal(t, 1, tables, "queries reach the database the migrations ran on")
}
