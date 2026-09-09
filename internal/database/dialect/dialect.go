// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package dialect

import (
	"strconv"
	"strings"
)

// Kind identifies the SQL dialect used by a connection.
type Kind int

const (
	// SQLite is the SQLite SQL dialect.
	SQLite Kind = iota
	// Postgres is the Postgres-protocol SQL dialect.
	Postgres
)

const (
	collateNocase  = "name COLLATE NOCASE ASC"
	lowerNameOrder = "LOWER(name) ASC"
	placeholderGrowExtra = 8
)

// NameOrder returns an ORDER BY expression for case-insensitive names.
func (kind Kind) NameOrder() string {
	if kind == Postgres {
		return lowerNameOrder
	}

	return collateNocase
}

// Rewrite translates SQL placeholders and SQLite collations for the dialect.
func (kind Kind) Rewrite(query string) string {
	if kind != Postgres {
		return query
	}

	return rewritePlaceholders(rewriteCollate(query))
}

func rewriteCollate(query string) string {
	return strings.ReplaceAll(query, collateNocase, lowerNameOrder)
}

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
