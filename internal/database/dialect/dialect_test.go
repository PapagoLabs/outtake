// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package dialect

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

	assert.Equal(t, collateNocase, SQLite.NameOrder())
	assert.Equal(t, lowerNameOrder, Postgres.NameOrder())
}

func TestRewrite_SQLiteUnchanged(t *testing.T) {
	t.Parallel()

	query := "SELECT * FROM clips WHERE id = ?"
	assert.Equal(t, query, SQLite.Rewrite(query))
}

func TestRewrite_Postgres(t *testing.T) {
	t.Parallel()

	got := Postgres.Rewrite("SELECT * FROM clips WHERE id = ? ORDER BY name COLLATE NOCASE ASC")
	assert.Equal(t, "SELECT * FROM clips WHERE id = $1 ORDER BY LOWER(name) ASC", got)
}
