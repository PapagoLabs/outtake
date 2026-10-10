// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"modernc.org/sqlite"

	sqlite3 "modernc.org/sqlite/lib"
)

// dialect identifies the SQL dialect used by a connection.
type dialect int

const (
	// dialectSQLite is the default dialect.
	dialectSQLite dialect = iota
	// dialectPostgres is the dialect spoken by the Postgres backend.
	dialectPostgres
)

// pgUniqueViolation is the Postgres error code for a unique constraint.
const pgUniqueViolation = "23505"

const (
	// BackendSQLite is the default libsql/SQLite backend.
	BackendSQLite = "sqlite"
	// BackendPostgres is the Postgres-protocol backend.
	BackendPostgres = "postgres"
	// BackendPgx is an alias for the Postgres-protocol backend.
	BackendPgx = "pgx"

	// collateNocase is how SQLite orders names case-insensitively.
	collateNocase = "name COLLATE NOCASE ASC"
	// lowerNameOrder is how Postgres orders names case-insensitively.
	lowerNameOrder = "LOWER(name) ASC"
)

const (
	// placeholderGrowExtra is the extra capacity reserved for numbered placeholders.
	placeholderGrowExtra = 8
)

// nameOrder returns an ORDER BY expression for case-insensitive names.
//
// Returns:
//   - clause: The ordering fragment for the dialect.
func (db *DB) nameOrder() string {
	if db.dialect == dialectPostgres {
		return lowerNameOrder
	}

	return collateNocase
}

// rewrite translates SQL placeholders and SQLite collations for the dialect.
//
// Parameters:
//   - query: The SQLite-shaped SQL to translate.
//
// Returns:
//   - translated: The query in the connection's dialect.
func (db *DB) rewrite(query string) string {
	if db.dialect != dialectPostgres {
		return query
	}

	return rewritePlaceholders(rewriteCollate(query))
}

// rewriteCollate replaces SQLite NOCASE ordering with a Postgres equivalent.
//
// Parameters:
//   - query: The query to scan for collation clauses.
//
// Returns:
//   - translated: The query with SQLite ordering replaced.
func rewriteCollate(query string) string {
	return strings.ReplaceAll(query, collateNocase, lowerNameOrder)
}

// rewritePlaceholders converts question-mark placeholders into numbered dollar form.
//
// Parameters:
//   - query: The query to scan for question-mark placeholders.
//
// Returns:
//   - translated: The query with placeholders numbered from one.
func rewritePlaceholders(query string) string {
	var builder strings.Builder

	builder.Grow(len(query) + placeholderGrowExtra)

	index := 1

	for idx := range len(query) {
		if query[idx] != '?' {
			_ = builder.WriteByte(query[idx])

			continue
		}

		_ = builder.WriteByte('$')
		_, _ = builder.WriteString(strconv.Itoa(index))

		index++
	}

	return builder.String()
}

// isUniqueViolation reports whether a write broke a unique constraint, read
// from the driver's error code rather than its message. The SQLite and
// Postgres drivers report a code. A libsql error carries none, so it reads as
// another failure.
//
// Parameters:
//   - err: The write's error, which may be nil.
//
// Returns:
//   - violated: True when a unique constraint refused the write.
func isUniqueViolation(err error) bool {
	if sqliteErr, ok := errors.AsType[*sqlite.Error](err); ok {
		return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
	}

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == pgUniqueViolation
	}

	return false
}
