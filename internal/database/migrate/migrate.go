// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"slices"

	"github.com/PapagoLabs/outtake/internal/database/dialect"
)

const sqlFileExtensionLen = 4

//go:embed migrations
var migrationsFS embed.FS

// Run creates the schema_migrations table and applies pending SQL files.
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//   - conn: Database handle.
//   - kind: Migration or dialect kind identifier.
//
// Returns:
//   - err: Non-nil when the migrations table cannot be prepared or pending migrations cannot be applied.
func Run(ctx context.Context, conn *sql.DB, kind dialect.Kind) error {
	_, err := conn.ExecContext(ctx, kind.Rewrite(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY
		)
	`))
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return fmt.Errorf("list applied migrations: %w", err)
	}

	err = applyPending(ctx, conn, kind, applied)
	if err != nil {
		return fmt.Errorf("apply pending: %w", err)
	}

	return nil
}

// appliedMigrations handles the HTTP request.
//
// Parameters:
//   - ctx: Fiber request/response for this HTTP handler.
//   - conn: Database handle.
//
// Returns:
//   - items: Result slice. Empty when none match.
//   - err: Non-nil when applied migrations cannot be listed or scanned.
func appliedMigrations(ctx context.Context, conn *sql.DB) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT name FROM schema_migrations`)
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

// applyPending handles the HTTP request.
//
// Parameters:
//   - ctx: Fiber request/response for this HTTP handler.
//   - conn: Database handle.
//   - kind: Migration or dialect kind identifier.
//   - applied: Already-applied migration ids.
//
// Returns:
//   - err: Non-nil when migration files cannot be read or a pending migration cannot be applied.
func applyPending(ctx context.Context, conn *sql.DB, kind dialect.Kind, applied []string) error {
	entries, err := migrationsFS.ReadDir(migrationsDir(kind))
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	for _, entry := range entries {
		if skipMigration(entry, applied) {
			continue
		}

		err := applyOne(ctx, conn, kind, entry.Name())
		if err != nil {
			return fmt.Errorf("apply pending: %w", err)
		}
	}

	return nil
}

// applyOne handles the HTTP request.
//
// Parameters:
//   - ctx: Fiber request/response for this HTTP handler.
//   - conn: Database handle.
//   - kind: Migration or dialect kind identifier.
//   - name: Display or lookup name.
//
// Returns:
//   - err: Non-nil when applying or recording a migration fails.
func applyOne(ctx context.Context, conn *sql.DB, kind dialect.Kind, name string) error {
	err := execMigration(ctx, conn, kind, name)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	_, err = conn.ExecContext(
		ctx,
		kind.Rewrite(`INSERT INTO schema_migrations (name) VALUES (?)`),
		name,
	)
	if err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}

	return nil
}

// execMigration handles the HTTP request.
//
// Parameters:
//   - ctx: Fiber request/response for this HTTP handler.
//   - conn: Database handle.
//   - kind: Migration or dialect kind identifier.
//   - name: Display or lookup name.
//
// Returns:
//   - err: Non-nil when a migration file cannot be read or executed.
func execMigration(ctx context.Context, conn *sql.DB, kind dialect.Kind, name string) error {
	content, err := migrationsFS.ReadFile(migrationsDir(kind) + "/" + name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}

	_, err = conn.ExecContext(ctx, string(content))
	if err != nil {
		return fmt.Errorf("exec migration %s: %w", name, err)
	}

	return nil
}

// migrationsDir returns the migrations dir.
//
// Parameters:
//   - kind: Migration or dialect kind identifier.
//
// Returns:
//   - value: The migrations dir.
func migrationsDir(kind dialect.Kind) string {
	if kind == dialect.Postgres {
		return "migrations/postgres"
	}

	return "migrations"
}

// isValidSQLFile reports whether valid sql file.
//
// Parameters:
//   - name: Display or lookup name.
//
// Returns:
//   - ok: True when valid sql file.
func isValidSQLFile(name string) bool {
	return len(name) >= sqlFileExtensionLen && name[len(name)-sqlFileExtensionLen:] == ".sql"
}

// skipMigration reports whether skip migration.
//
// Parameters:
//   - entry: Directory entry for a migration file.
//   - applied: Already-applied migration ids.
//
// Returns:
//   - ok: True when skip migration.
func skipMigration(entry fs.DirEntry, applied []string) bool {
	return entry.IsDir() || !isValidSQLFile(entry.Name()) || slices.Contains(applied, entry.Name())
}
