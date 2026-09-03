// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRewritePlaceholders(t *testing.T) {
	t.Parallel()

	got := rewritePlaceholders("SELECT a FROM t WHERE id = ? AND b = ?")
	assert.Equal(t, "SELECT a FROM t WHERE id = $1 AND b = $2", got)
}

func TestRewriteCollate(t *testing.T) {
	t.Parallel()

	got := rewriteCollate("ORDER BY is_default DESC, name COLLATE NOCASE ASC")
	assert.Equal(t, "ORDER BY is_default DESC, LOWER(name) ASC", got)
}

func TestNameOrder(t *testing.T) {
	t.Parallel()

	sqlite := &DB{conn: nil, dialect: dialectSQLite}
	assert.Equal(t, collateNocase, sqlite.nameOrder())

	postgres := &DB{conn: nil, dialect: dialectPostgres}
	assert.Equal(t, lowerNameOrder, postgres.nameOrder())
}

func TestDBRewrite_SQLiteUnchanged(t *testing.T) {
	t.Parallel()

	db := &DB{conn: nil, dialect: dialectSQLite}
	query := "SELECT * FROM clips WHERE id = ?"
	assert.Equal(t, query, db.rewrite(query))
}

func TestDBRewrite_Postgres(t *testing.T) {
	t.Parallel()

	db := &DB{conn: nil, dialect: dialectPostgres}
	got := db.rewrite("SELECT * FROM clips WHERE id = ? ORDER BY name COLLATE NOCASE ASC")
	assert.Equal(t, "SELECT * FROM clips WHERE id = $1 ORDER BY LOWER(name) ASC", got)
}
