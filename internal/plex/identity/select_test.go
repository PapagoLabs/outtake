// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity/mocks"
)

// errStoreClosed reports a store that cannot be written to.
var errStoreClosed = errors.New("database is closed")

func TestSelectBindsAndPersists(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveSelectedServer(mock.Anything, plex.Server{Name: "Attic"}).
		Return(nil).
		Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Set(plex.Server{Name: "Attic"}).
		Once()

	auth := New("outtake", "test", "http://localhost", store, bound)

	require.NoError(t, auth.Select(t.Context(), plex.Server{Name: "Attic"}))
}

func TestSelectReportsAFailedWrite(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveSelectedServer(mock.Anything, mock.Anything).
		Return(errStoreClosed)

	auth := New("outtake", "test", "http://localhost", store, nil)

	err := auth.Select(t.Context(), plex.Server{Name: "Attic"})
	require.ErrorContains(t, err, "save selected server")
}

func TestSelectToleratesAbsentCollaborators(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test", "http://localhost", nil, nil)

	require.NoError(t, auth.Select(t.Context(), plex.Server{Name: "Attic"}))
}

func TestGenerateClientIDIsRandom(t *testing.T) {
	t.Parallel()

	first := GenerateClientID()
	second := GenerateClientID()

	assert.Len(t, first, 2*ClientIDLength)
	assert.NotEqual(t, first, second)
}
