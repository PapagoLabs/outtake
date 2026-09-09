// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package database provides SQLite and Postgres-protocol access for outtake.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql" // LibSQL driver.
	_ "modernc.org/sqlite"                               // SQLite driver.

	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database/dialect"
	"github.com/PapagoLabs/outtake/internal/database/migrate"
)

// DB represents a database connection.
type DB struct {
	conn    *sql.DB
	dialect dialect.Kind
}

const (
	// migrateTimeout is the maximum time allowed to ping and apply migrations.
	migrateTimeout = 30 * time.Second
)

var (
	// errUnknownDatabaseBackend is returned when the database backend is unknown.
	errUnknownDatabaseBackend = errors.New("unknown database backend")
	// errDatabaseURLRequired is returned when postgres is selected without a DSN.
	errDatabaseURLRequired = errors.New("database-url is required")
)

const (
	// BackendSQLite is the default libsql/SQLite backend.
	BackendSQLite = "sqlite"
	// BackendPostgres is the Postgres-protocol backend.
	BackendPostgres = "postgres"
	// BackendPgx is an alias for the Postgres-protocol backend.
	BackendPgx = "pgx"
)

// New creates a new SQLite database instance.
//
// Parameters:
//   - dbPath: Typed string argument for New.
//
// Returns:
//   - db: A new SQLite database instance.
//   - err: Wrapped failure such as "open database"; "open sqlite".
func New(dbPath string) (*DB, error) {
	dsn := "file:" + filepath.Clean(dbPath)
	conn, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	db, err := finishOpen(conn, dialect.SQLite)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	return db, nil
}

// NewFromConfig constructs the database backend selected by configuration.
//
// Parameters:
//   - cfg: Application configuration.
//
// Returns:
//   - db: The database backend selected by configuration.
//   - err: Wrapped failure such as "sqlite database"; "postgres database";
//     "...: ...".
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
//
// Returns:
//   - err: Wrapped failure from "close database".
func (db *DB) Close() error {
	err := db.conn.Close()
	if err != nil {
		return fmt.Errorf("close database: %w", err)
	}

	return nil
}

// Conn returns the underlying database connection.
//
// Returns:
//   - db: The underlying database connection.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// nameOrder returns an ORDER BY expression for case-insensitive names.
//
// Returns:
//   - value: An ORDER BY expression for case-insensitive names.
func (db *DB) nameOrder() string {
	return db.dialect.NameOrder()
}

// rewrite translates SQL placeholders and SQLite collations for the dialect.
//
// Parameters:
//   - query: Search or filter query string.
//
// Returns:
//   - value: Result value; zero or empty when unavailable.
func (db *DB) rewrite(query string) string {
	return db.dialect.Rewrite(query)
}

// finishOpen pings and migrates a newly opened connection.
//
// Parameters:
//   - conn: Database handle.
//   - kind: Migration or dialect kind identifier.
//
// Returns:
//   - db: The database handle.
//   - err: Wrapped failure such as "ping database"; "migrate".
func finishOpen(conn *sql.DB, kind dialect.Kind) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), migrateTimeout)
	defer cancel()

	err := conn.PingContext(ctx)
	if err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("ping database: %w", err)
	}

	err = migrate.Run(ctx, conn, kind)
	if err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &DB{conn: conn, dialect: kind}, nil
}
