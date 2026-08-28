// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package database provides SQLite database access for outtake.
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	// LibSQL driver.
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	// SQLite driver.
	_ "modernc.org/sqlite"
)

// DB represents a database connection.
type DB struct {
	conn *sql.DB
}

// sqlFileExtensionLen is the length of the SQL file extension.
const sqlFileExtensionLen = 4

// migrationsFS contains the migration files.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// New creates a new database instance.
func New(dbPath string) (*DB, error) {
	dsn := "file:" + filepath.Clean(dbPath)
	conn, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = conn.PingContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{conn: conn}

	err = db.migrate()
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	err := db.conn.Close()
	if err != nil {
		return fmt.Errorf("close database: %w", err)
	}

	return nil
}

// Conn returns the underlying database connection.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// isValidSQLFile checks if a filename has a valid SQL extension.
func isValidSQLFile(name string) bool {
	return len(name) >= sqlFileExtensionLen && name[len(name)-sqlFileExtensionLen:] == ".sql"
}

// appliedMigrations returns migration filenames already recorded.
func (db *DB) appliedMigrations(ctx context.Context) ([]string, error) {
	rows, err := db.conn.QueryContext(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	defer rows.Close()

	names := make([]string, 0)

	for rows.Next() {
		var name string

		scanErr := rows.Scan(&name)
		if scanErr != nil {
			return nil, fmt.Errorf("scan migration: %w", scanErr)
		}

		names = append(names, name)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate migrations: %w", err)
	}

	return names, nil
}

// applyOne executes and records a single migration file.
func (db *DB) applyOne(ctx context.Context, name string) error {
	err := db.execMigration(ctx, name)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	_, err = db.conn.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (name) VALUES (?)`,
		name,
	)
	if err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}

	return nil
}

// applyPending runs SQL files that are not yet recorded.
func (db *DB) applyPending(ctx context.Context, applied []string) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	for _, entry := range entries {
		if skipMigration(entry, applied) {
			continue
		}

		err := db.applyOne(ctx, entry.Name())
		if err != nil {
			return fmt.Errorf("apply pending: %w", err)
		}
	}

	return nil
}

// execMigration executes a single migration file.
func (db *DB) execMigration(ctx context.Context, name string) error {
	content, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}

	_, err = db.conn.ExecContext(ctx, string(content))
	if err != nil {
		return fmt.Errorf("exec migration %s: %w", name, err)
	}

	return nil
}

// migrate runs the database migrations.
func (db *DB) migrate() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := db.conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := db.appliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("list applied migrations: %w", err)
	}

	err = db.applyPending(ctx, applied)
	if err != nil {
		return fmt.Errorf("apply pending: %w", err)
	}

	return nil
}

// skipMigration reports whether a directory entry should not be applied.
func skipMigration(entry fs.DirEntry, applied []string) bool {
	return entry.IsDir() || !isValidSQLFile(entry.Name()) || slices.Contains(applied, entry.Name())
}
