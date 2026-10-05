// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"testing"

	"github.com/stretchr/testify/require"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

func TestNew(t *testing.T) {
	t.Parallel()

	db, err := New(t.TempDir() + "/test.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	err = db.Conn().PingContext(t.Context())
	require.NoError(t, err)
}
