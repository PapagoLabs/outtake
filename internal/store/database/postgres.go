// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PGX Postgres-protocol driver.
)

const (
	// postgresMaxOpenConns is how many open connections Postgres allows.
	postgresMaxOpenConns = 25
	// postgresMaxIdleConns is how many connections Postgres keeps idle.
	postgresMaxIdleConns = 5
	// postgresConnMaxLifetime is the lifetime after which a connection is retired.
	postgresConnMaxLifetime = 5 * time.Minute
)

// newPostgres opens a Postgres-protocol database using pgx.
//
// Parameters:
//   - dsn: The Postgres connection string.
//
// Returns:
//   - db: A migrated database handle.
//   - err: Non-nil when the database cannot be opened or migrated.
func newPostgres(dsn string) (*DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(postgresMaxOpenConns)
	conn.SetMaxIdleConns(postgresMaxIdleConns)
	conn.SetConnMaxLifetime(postgresConnMaxLifetime)

	db, err := finishOpen(conn, dialectPostgres)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	return db, nil
}
