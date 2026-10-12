// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/library/mocks"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// sharedPMS stands in for a Plex server that shares one item with a member.
//
// Item 42 answers, any other item answers 404, and item 500 fails.
//
// Parameters:
//   - t: The test the server belongs to.
//
// Returns:
//   - server: The stand-in as a connection.
func sharedPMS(t *testing.T) plex.Server {
	t.Helper()

	stub := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/library/metadata/42":
				writer.Header().Set("Content-Type", "application/json")

				_, _ = writer.Write(
					[]byte(
						`{"MediaContainer":{"Metadata":[{"ratingKey":"42","title":"Shared","type":"movie"}]}}`,
					),
				)
			case "/library/metadata/500":
				writer.WriteHeader(http.StatusInternalServerError)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		}),
	)
	t.Cleanup(stub.Close)

	server, ok := plex.ParseServerURL(stub.URL)
	require.True(t, ok)

	server.Token = "member-token"

	return server
}

// TestCheckAccessFollowsWhatTheServerShares covers the check before a member's
// render.
//
// The owner is never asked about. A member may render an item the server
// shares with them, not one it does not, and nothing without a token for the
// server. A server that cannot answer refuses the render too.
func TestCheckAccessFollowsWhatTheServerShares(t *testing.T) {
	t.Parallel()

	server := sharedPMS(t)
	client := plex.NewClient(
		plex.ClientConfig{Product: "outtake", ClientID: "test", Token: "member-token"},
	)

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().ClientWithToken("member-token").Return(client, server, true)

	source := NewMediaSource(&config.Config{}, selected, nil)
	member := identity.Viewer{Token: "member-token", PlexID: 7, Owner: false}

	require.NoError(t, source.CheckAccess(t.Context(), identity.OwnerViewer(42), "99"),
		"the owner sees every item")
	require.NoError(t, source.CheckAccess(t.Context(), member, "42"))
	require.ErrorIs(t, source.CheckAccess(t.Context(), member, "99"), ErrNotShared)
	require.ErrorIs(
		t,
		source.CheckAccess(t.Context(), identity.Viewer{Token: "", PlexID: 7, Owner: false}, "42"),
		ErrNotShared,
		"an account with no token for the server sees nothing",
	)

	err := source.CheckAccess(t.Context(), member, "500")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotShared, "a failure to ask is not a refusal by Plex")
}

// TestCheckAccessWithoutAServerSelection covers a source with no server
// behind it, which refuses a member rather than reading as the owner.
func TestCheckAccessWithoutAServerSelection(t *testing.T) {
	t.Parallel()

	source := NewMediaSource(&config.Config{}, nil, nil)

	require.ErrorIs(
		t,
		source.CheckAccess(
			t.Context(),
			identity.Viewer{Token: "member-token", PlexID: 7, Owner: false},
			"42",
		),
		ErrNotShared,
	)
}

// TestCheckAccessAsksPlexWithTheMembersToken covers which token reaches the
// server behind a real binding that holds the owner's.
//
// The item lookup carries the member's token, never the owner's, so Plex
// answers with what it shares with the member.
func TestCheckAccessAsksPlexWithTheMembersToken(t *testing.T) {
	t.Parallel()

	tokens := make(chan string, 4)

	stub := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/library/metadata/42" {
				tokens <- request.Header.Get("X-Plex-Token")

				writer.Header().Set("Content-Type", "application/json")

				_, _ = writer.Write(
					[]byte(
						`{"MediaContainer":{"Metadata":[{"ratingKey":"42","title":"Shared","type":"movie"}]}}`,
					),
				)

				return
			}

			writer.WriteHeader(http.StatusNotFound)
		}),
	)
	t.Cleanup(stub.Close)

	server, ok := plex.ParseServerURL(stub.URL)
	require.True(t, ok)

	server.Token = "owner-token"

	bind := identity.NewBinding("outtake", "test", time.Hour)
	bind.Set(server)
	t.Cleanup(bind.Stop)

	source := NewMediaSource(&config.Config{}, bind, nil)

	require.NoError(t, source.CheckAccess(
		t.Context(),
		identity.Viewer{Token: "member-token", PlexID: 7, Owner: false},
		"42",
	))
	assert.Equal(t, "member-token", <-tokens)
}

// TestCheckAccessRefusesAServerCarryingAnotherToken covers a selection that
// returns a server with a token other than the viewer's, which is refused
// rather than asked with.
func TestCheckAccessRefusesAServerCarryingAnotherToken(t *testing.T) {
	t.Parallel()

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().
		ClientWithToken("member-token").
		Return(plex.NewClient(plex.ClientConfig{}), plex.Server{Token: "owner-token"}, true)

	source := NewMediaSource(&config.Config{}, selected, nil)

	require.ErrorIs(
		t,
		source.CheckAccess(
			t.Context(),
			identity.Viewer{Token: "member-token", PlexID: 7, Owner: false},
			"42",
		),
		ErrNotShared,
	)
}
