// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package database provides SQLite and Postgres-protocol access for outtake.
package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql" // LibSQL driver.
	_ "modernc.org/sqlite"                               // SQLite driver.

	"github.com/PapagoLabs/outtake/internal/config"
)

// DB represents a database connection.
type DB struct {
	conn    *sql.DB
	dialect dialect
}

const (
	// SQLFileExtensionLen is the length of the SQL file extension.
	sqlFileExtensionLen = 4

	// MigrateTimeout is the maximum time allowed to ping and apply migrations.
	migrateTimeout = 30 * time.Second
)

var (
	// ErrUnknownDatabaseBackend is returned when the database backend is unknown.
	errUnknownDatabaseBackend = errors.New("unknown database backend")
	// ErrDatabaseURLRequired is returned when postgres is selected without a DSN.
	errDatabaseURLRequired = errors.New("database-url is required")
)

// migrationsFS contains the migration files.
//
//go:embed migrations
var migrationsFS embed.FS

// New creates a new SQLite database instance.
func New(dbPath string) (*DB, error) {
	dsn := "file:" + filepath.Clean(dbPath)
	conn, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	db, err := finishOpen(conn, dialectSQLite)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	return db, nil
}

// NewFromConfig constructs the database backend selected by configuration.
func NewFromConfig(cfg *config.Config) (*DB, error) {
	backend := strings.ToLower(strings.TrimSpace(cfg.DatabaseBackend))
	switch backend {
	case "", BackendSQLite, "libsql":
		db, err := New(cfg.DatabasePath)
		if err != nil {
			return nil, fmt.Errorf("sqlite database: %w", err)
		}

		return db, nil
	case BackendPostgres, BackendPgx, "postgresql":
		if strings.TrimSpace(cfg.DatabaseURL) == "" {
			return nil, errDatabaseURLRequired
		}

		db, err := newPostgres(cfg.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("postgres database: %w", err)
		}

		return db, nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnknownDatabaseBackend, cfg.DatabaseBackend)
	}
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
		db.rewrite(`INSERT INTO schema_migrations (name) VALUES (?)`),
		name,
	)
	if err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}

	return nil
}

// applyPending runs SQL files that are not yet recorded.
func (db *DB) applyPending(ctx context.Context, applied []string) error {
	entries, err := migrationsFS.ReadDir(db.migrationsDir())
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
	content, err := migrationsFS.ReadFile(db.migrationsDir() + "/" + name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}

	_, err = db.conn.ExecContext(ctx, string(content))
	if err != nil {
		return fmt.Errorf("exec migration %s: %w", name, err)
	}

	return nil
}

// finishOpen pings and migrates a newly opened connection.
func finishOpen(conn *sql.DB, sqlDialect dialect) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), migrateTimeout)
	defer cancel()

	err := conn.PingContext(ctx)
	if err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{conn: conn, dialect: sqlDialect}

	err = db.migrate(ctx)
	if err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// migrate runs the database migrations.
func (db *DB) migrate(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, db.rewrite(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY
		)
	`))
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

// migrationsDir returns the embed directory for this dialect.
func (db *DB) migrationsDir() string {
	if db.dialect == dialectPostgres {
		return "migrations/postgres"
	}

	return "migrations"
}

// isValidSQLFile checks if a filename has a valid SQL extension.
func isValidSQLFile(name string) bool {
	return len(name) >= sqlFileExtensionLen && name[len(name)-sqlFileExtensionLen:] == ".sql"
}

// skipMigration reports whether a directory entry should not be applied.
func skipMigration(entry fs.DirEntry, applied []string) bool {
	return entry.IsDir() || !isValidSQLFile(entry.Name()) || slices.Contains(applied, entry.Name())
}
