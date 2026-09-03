// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"strconv"
	"strings"
)

// dialect identifies the SQL dialect used by a connection.
type dialect int

const (
	dialectSQLite dialect = iota
	dialectPostgres
)

const (
	// BackendSQLite is the default libsql/SQLite backend.
	BackendSQLite = "sqlite"
	// BackendPostgres is the Postgres-protocol backend.
	BackendPostgres = "postgres"
	// BackendPgx is an alias for the Postgres-protocol backend.
	BackendPgx = "pgx"

	collateNocase  = "name COLLATE NOCASE ASC"
	lowerNameOrder = "LOWER(name) ASC"
)

// nameOrder returns an ORDER BY expression for case-insensitive names.
func (db *DB) nameOrder() string {
	if db.dialect == dialectPostgres {
		return lowerNameOrder
	}

	return collateNocase
}

// rewrite translates SQL placeholders and SQLite collations for the dialect.
func (db *DB) rewrite(query string) string {
	if db.dialect != dialectPostgres {
		return query
	}

	return rewritePlaceholders(rewriteCollate(query))
}

// rewriteCollate replaces SQLite NOCASE ordering with a Postgres equivalent.
func rewriteCollate(query string) string {
	return strings.ReplaceAll(query, collateNocase, lowerNameOrder)
}

// rewritePlaceholders converts ? placeholders into $1, $2, ...
func rewritePlaceholders(query string) string {
	var builder strings.Builder

	builder.Grow(len(query) + 8)

	index := 1
	for idx := 0; idx < len(query); idx++ {
		if query[idx] != '?' {
			builder.WriteByte(query[idx])

			continue
		}

		builder.WriteByte('$')
		builder.WriteString(strconv.Itoa(index))
		index++
	}

	return builder.String()
}
