// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"database/sql"
	"fmt"
	"time"

	// PGX Postgres-protocol driver.
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	postgresMaxOpenConns    = 25
	postgresMaxIdleConns    = 5
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

	return finishOpen(conn, dialectPostgres)
}
