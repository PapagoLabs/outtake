// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/database"
)

func TestResolveQuality(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/quality.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	handler := &ClipHandler{
		clipQueue:   nil,
		clipStorage: nil,
		db:          db,
		cfg:         nil,
		bind:        nil,
		product:     "",
		clientID:    "",
	}

	id, err := handler.resolveQuality(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, "medium", id)

	id, err = handler.resolveQuality(t.Context(), "high")
	require.NoError(t, err)
	assert.Equal(t, "high", id)

	_, err = handler.resolveQuality(t.Context(), "nope")
	require.ErrorIs(t, err, errUnknownQuality)
}
