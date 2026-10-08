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

	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// DB represents a database connection.
type DB struct {
	conn    *sql.DB
	dialect dialect
}

const (
	// sqlFileExtensionLen is the length of the extension check, counting the leading dot.
	sqlFileExtensionLen = 4

	// migrateTimeout is the deadline pinging and applying migrations share.
	migrateTimeout = 30 * time.Second
)

// SQLite connection settings.
const (
	// sqliteOptions configure every pooled connection. WAL lets readers run
	// beside the one writer, busy_timeout makes a writer wait for the lock
	// rather than fail, synchronous=NORMAL is durable enough under WAL, and an
	// immediate transaction takes the write lock up front, so two writers
	// never deadlock upgrading a read lock.
	sqliteOptions = "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)&_txlock=immediate"

	// sqliteConns is how many connections the pool keeps open.
	sqliteConns = 4
)

var (
	// errUnknownDatabaseBackend rejects an unrecognized database backend.
	errUnknownDatabaseBackend = errors.New("unknown database backend")
	// errDatabaseURLRequired reports Postgres without a DSN.
	errDatabaseURLRequired = errors.New("database-url is required")
)

// migrationsFS contains the migration files.
//
//go:embed migrations
var migrationsFS embed.FS

// New creates a new SQLite database instance.
//
// Parameters:
//   - dbPath: Path to the SQLite database file.
//
// Returns:
//   - db: A migrated database handle.
//   - err: Non-nil when the database cannot be opened or migrated.
func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("libsql", "file:"+filepath.Clean(dbPath)+sqliteOptions)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Each connection to an in-memory database opens a database of its own, so
	// one held open is the only way every query sees the same tables.
	conns := sqliteConns
	if strings.Contains(dbPath, ":memory:") || strings.Contains(dbPath, "mode=memory") {
		conns = 1
	}

	conn.SetMaxOpenConns(conns)
	conn.SetMaxIdleConns(conns)

	db, err := finishOpen(conn, dialectSQLite)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	return db, nil
}

// NewFromConfig constructs the database backend selected by configuration.
//
// Parameters:
//   - cfg: The loaded configuration naming the backend.
//
// Returns:
//   - db: A migrated database handle.
//   - err: Non-nil when the backend is unknown, unconfigured, or cannot be opened.
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
//   - err: Non-nil when the connection cannot be closed.
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
//   - conn: The pooled connection this database queries through.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// appliedMigrations returns migration filenames already recorded.
//
// Parameters:
//   - ctx: Request scope for the query.
//
// Returns:
//   - names: The recorded migration filenames.
//   - err: Non-nil when the ledger cannot be read.
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
//
// Parameters:
//   - ctx: Request scope for the migration.
//   - name: Migration filename inside the embedded directory.
//
// Returns:
//   - err: Non-nil when the migration fails or cannot be recorded.
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
//
// Parameters:
//   - ctx: Request scope for the migrations.
//   - applied: Migration filenames already recorded.
//
// Returns:
//   - err: Non-nil when the migration directory cannot be read or a file fails.
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
//
// Parameters:
//   - ctx: Request scope for the migration.
//   - name: Migration filename inside the embedded directory.
//
// Returns:
//   - err: Non-nil when the file cannot be read or executed.
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
//
// Parameters:
//   - conn: The freshly opened connection.
//   - sqlDialect: The dialect the connection speaks.
//
// Returns:
//   - db: A migrated database handle.
//   - err: Non-nil when the ping or migration fails.
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
//
// Parameters:
//   - ctx: Request scope carrying the migration deadline.
//
// Returns:
//   - err: Non-nil when the ledger cannot be created or a migration fails.
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
//
// Returns:
//   - dir: The embedded path holding this dialect's migrations.
func (db *DB) migrationsDir() string {
	if db.dialect == dialectPostgres {
		return "migrations/postgres"
	}

	return "migrations"
}

// isValidSQLFile checks if a filename has a valid SQL extension.
//
// Parameters:
//   - name: Filename from the embedded migrations.
//
// Returns:
//   - valid: True when the filename ends in .sql.
func isValidSQLFile(name string) bool {
	return len(name) >= sqlFileExtensionLen && name[len(name)-sqlFileExtensionLen:] == ".sql"
}

// skipMigration reports whether a directory entry should not be applied.
//
// Parameters:
//   - entry: A directory entry from the embedded migrations.
//   - applied: Migration filenames already recorded.
//
// Returns:
//   - skip: True for a directory, a non-SQL file, or an applied migration.
func skipMigration(entry fs.DirEntry, applied []string) bool {
	return entry.IsDir() || !isValidSQLFile(entry.Name()) || slices.Contains(applied, entry.Name())
}
