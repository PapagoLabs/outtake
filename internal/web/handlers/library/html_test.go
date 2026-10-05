// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/library/mocks"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestPlexPairHandsBackTheSelectedServer(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"},
		{"key":"2","title":"TV Shows","type":"show"}
	]}}`))
	})

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	client, server, ok := handler.plexPair()

	require.True(t, ok, "a bound server has to be reachable through the same pair")
	assert.NotNil(t, client)
	assert.Equal(t, stub.server, server)
}

func TestPlexPairReportsNoSelectedServer(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	client, server, ok := handler.plexPair()

	assert.False(t, ok)
	assert.Nil(t, client)
	assert.Equal(t, plex.EmptyServer(), server)
}

func TestSidebarLibrariesListsEveryLibrary(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/sections/all", r.URL.Path)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"},
		{"key":"2","title":"TV Shows","type":"show"}
	]}}`))
	})

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	assert.Equal(t, []view.LibraryItem{
		{ID: "1", Title: "Movies", Type: "movie"},
		{ID: "2", Title: "TV Shows", Type: "show"},
	}, sidebarLibrariesOf(t, handler),
		"both sections are what the sidebar has to offer")
}

func TestSidebarLibrariesReportsNothingWithNoServerBound(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	assert.Nil(t, sidebarLibrariesOf(t, handler),
		"with no server there is no library to link to")
}

func TestSidebarLibrariesReportsNothingWhenPlexIsUnreachable(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	assert.Nil(t, sidebarLibrariesOf(t, handler),
		"an unreachable Plex leaves the sidebar empty rather than half filled")
}

func TestSidebarLibrariesReportsNothingOnAMalformedResponse(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":"not-a-list"}}`))
	})

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	assert.Nil(t, sidebarLibrariesOf(t, handler))
}

func TestNavLibrariesRendersTheSidebarFragment(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"},
		{"key":"2","title":"TV Shows","type":"show"}
	]}}`))
	})

	auth := offlineAuthWithStub(t, stub)

	handler, _ := pageHandler(t, auth, silentSources(t))

	app := fiber.New()
	app.Get("/nav/libraries", handler.NavLibraries)

	answer := serve(t, app, "/nav/libraries", true)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Movies",
		"a library the sidebar links to has to be in the fragment")
	assertBodyContains(t, answer.body, "TV Shows",
		"every section the server reported is offered")
}

func TestNavLibrariesRendersNothingWithNoServerBound(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	app := fiber.New()
	app.Get("/nav/libraries", handler.NavLibraries)

	answer := serve(t, app, "/nav/libraries", true)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyOmits(t, answer.body, "Movies",
		"an empty library list swaps out cleanly instead of leaving stale links")
}

// sidebarLibrariesOf reads the sidebar libraries through a live request context.
//
// Parameters:
//   - t: The test the read belongs to.
//   - handler: The handler under test.
//
// Returns:
//   - libraries: The library models the sidebar would render.
func sidebarLibrariesOf(t *testing.T, handler *Handler) []view.LibraryItem {
	t.Helper()

	var libraries []view.LibraryItem

	app := fiber.New()
	app.Get("/nav/libraries", func(ctx fiber.Ctx) error {
		libraries = handler.sidebarLibraries(ctx)

		return ctx.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/nav/libraries", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	return libraries
}

// offlineAuthWithStub builds an authentication bound to a loopback Plex server.
//
// Parameters:
//   - t: The test the authentication belongs to.
//   - stub: The Plex Media Server the authentication is bound to.
//
// Returns:
//   - auth: The authentication under test.
func offlineAuthWithStub(
	t *testing.T,
	stub *pmsStub,
) *mocks.MockPlexAuth {
	t.Helper()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Client().Return(stub.client, stub.server, true).Maybe()

	return auth
}
