// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PGX Postgres-protocol driver.

	"github.com/PapagoLabs/outtake/internal/database/dialect"
)

const (
	// postgresMaxOpenConns is the maximum open connections for Postgres.
	postgresMaxOpenConns = 25
	// postgresMaxIdleConns is the maximum idle connections for Postgres.
	postgresMaxIdleConns = 5
	// postgresConnMaxLifetime is the maximum connection lifetime for Postgres.
	postgresConnMaxLifetime = 5 * time.Minute
)

// newPostgres opens a Postgres-protocol database using pgx.
//
// Parameters:
//   - dsn: Database connection URL/DSN.
//
// Returns:
//   - db: A Postgres-protocol database using pgx.
//   - err: Wrapped failure such as "open database"; "open postgres".
func newPostgres(dsn string) (*DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(postgresMaxOpenConns)
	conn.SetMaxIdleConns(postgresMaxIdleConns)
	conn.SetConnMaxLifetime(postgresConnMaxLifetime)

	db, err := finishOpen(conn, dialect.Postgres)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	return db, nil
}
