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
	// PostgresMaxOpenConns is the maximum open connections for Postgres.
	postgresMaxOpenConns = 25
	// PostgresMaxIdleConns is the maximum idle connections for Postgres.
	postgresMaxIdleConns = 5
	// PostgresConnMaxLifetime is the maximum connection lifetime for Postgres.
	postgresConnMaxLifetime = 5 * time.Minute
)

// newPostgres opens a Postgres-protocol database using pgx.
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
