// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/database/dialect"
)

func TestDBRewrite_SQLiteUnchanged(t *testing.T) {
	t.Parallel()

	db := &DB{conn: nil, dialect: dialect.SQLite}
	query := "SELECT * FROM clips WHERE id = ?"
	assert.Equal(t, query, db.rewrite(query))
}

func TestDBRewrite_Postgres(t *testing.T) {
	t.Parallel()

	db := &DB{conn: nil, dialect: dialect.Postgres}
	got := db.rewrite("SELECT * FROM clips WHERE id = ? ORDER BY name COLLATE NOCASE ASC")
	assert.Equal(t, "SELECT * FROM clips WHERE id = $1 ORDER BY LOWER(name) ASC", got)
}

func TestDBNameOrder(t *testing.T) {
	t.Parallel()

	sqlite := &DB{conn: nil, dialect: dialect.SQLite}
	assert.Equal(t, dialect.SQLite.NameOrder(), sqlite.nameOrder())

	postgres := &DB{conn: nil, dialect: dialect.Postgres}
	assert.Equal(t, dialect.Postgres.NameOrder(), postgres.nameOrder())
}
