// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
)

func newTestClient(t *testing.T, server *httptest.Server) *plex.Client {
	t.Helper()

	return plex.NewClient(plex.ClientConfig{
		Product:  "outtake",
		ClientID: "test-client",
		Token:    "test-token",
		Timeout:  5 * time.Second,
		BaseURL:  server.URL,
	})
}

func newTestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()

	return newStatusServer(t, http.StatusOK, body)
}

func newStatusServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(status)

		if body != "" {
			_, _ = writer.Write([]byte(body))
		}
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return server
}

func testPMS(t *testing.T, server *httptest.Server) plex.Server {
	t.Helper()

	parsed, ok := plex.ServerFromURL(server.URL, "test-token")
	require.True(t, ok, "the test server URL must parse into a Plex server")

	return parsed
}

func testPMSForURL(t *testing.T, baseURL string) plex.Server {
	t.Helper()

	parsed, ok := plex.ServerFromURL(baseURL, "test-token")
	require.True(t, ok, "the URL must parse into a Plex server")

	return parsed
}

func deadServerURL(t *testing.T) string {
	t.Helper()

	listenConfig := &net.ListenConfig{}

	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	require.NoError(t, listener.Close())

	return "http://" + net.JoinHostPort("127.0.0.1", port)
}

func TestIntegration_NewClient(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, ""))
	assert.Equal(t, "outtake", c.Product)
	assert.Equal(t, "test-client", c.ClientID)
}

func TestIntegration_SetToken(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, ""))
	c.SetToken("new-token")
	assert.Equal(t, "new-token", c.Token)
}

func TestIntegration_SetBaseURL(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, ""))
	require.NoError(t, c.SetBaseURL("http://192.168.1.100:32400"))
}

func TestIntegration_SetBaseURL_Invalid(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, ""))
	assert.Error(t, c.SetBaseURL("://invalid"))
}

func TestIntegration_GeneratePIN(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, `{"id": 12345, "code": "abc123"}`))

	pin, err := c.GeneratePIN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 12345, pin.ID)
	assert.Equal(t, "abc123", pin.Code)
}

func TestIntegration_GeneratePIN_ServerError(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newStatusServer(t, http.StatusInternalServerError, "internal error"))

	_, err := c.GeneratePIN(t.Context())
	assert.Error(t, err)
}

func TestIntegration_PollPIN(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, `{"authToken": "token-xyz"}`))

	token, err := c.PollPIN(t.Context(), 12345, "testpin")
	require.NoError(t, err)
	assert.Equal(t, "token-xyz", token)
}

func TestIntegration_PollPIN_NotClaimed(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, `{"authToken": ""}`))

	token, err := c.PollPIN(t.Context(), 12345, "testpin")
	require.ErrorIs(t, err, plex.ErrPINNotYetClaimed)
	assert.Empty(t, token)
}

func TestIntegration_ValidateToken(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, `{"id": 1, "title": "Test User"}`))

	valid, user, err := c.ValidateToken(t.Context())
	require.NoError(t, err)
	assert.True(t, valid)
	assert.Equal(t, "Test User", user.Title)
}

func TestIntegration_ValidateToken_Invalid(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newStatusServer(t, http.StatusUnauthorized, "unauthorized"))

	valid, _, err := c.ValidateToken(t.Context())
	require.ErrorIs(t, err, plex.ErrUnauthorized)
	assert.False(t, valid)
}

func TestIntegration_GetAuthURL(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, ""))

	authURL := c.GetAuthURL("pin-code", "my-client", "http://localhost:8080/callback")
	assert.True(t, strings.HasPrefix(authURL, "https://app.plex.tv/auth#?"))
	assert.NotContains(t, authURL, "#%3F")
	assert.Contains(t, authURL, "clientID=my-client")
	assert.Contains(t, authURL, "code=pin-code")
}

func TestIntegration_GetLibraries(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, `{"MediaContainer":{"Directory":[
			{"key":"1","title":"Movies","type":"movie"},
			{"key":"2","title":"TV Shows","type":"show"}
		]}}`)

	c := newTestClient(t, server)

	libs, err := c.GetLibraries(t.Context(), testPMS(t, server))
	require.NoError(t, err)
	assert.Len(t, libs, 2)
	assert.Equal(t, "Movies", libs[0].Title)
	assert.Equal(t, "TV Shows", libs[1].Title)
}

func TestIntegration_GetMedia(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, `{"MediaContainer":{"Metadata":[
			{"ratingKey":"100","title":"Test Movie","duration":7200000,"thumb":"/library/metadata/100/thumb/1","type":"movie"}
		]}}`)

	c := newTestClient(t, server)

	items, err := c.GetMedia(t.Context(), testPMS(t, server), "1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.InEpsilon(t, 7200.0, items[0].Duration, 0.01)
	assert.Equal(t, "Test Movie", items[0].Title)
}

func TestIntegration_GetMediaPath(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, `{"MediaContainer":{"Metadata":[
			{"Media":[{"Part":[{"file":"/media/movies/Test Movie.mkv"}]}]}
		]}}`)

	c := newTestClient(t, server)

	path, err := c.GetMediaPath(t.Context(), testPMS(t, server), "100")
	require.NoError(t, err)
	assert.Equal(t, "/media/movies/Test Movie.mkv", path)
}

func TestIntegration_GetMediaPath_NotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, `{"MediaContainer":{"Metadata":[{"Media":[{"Part":[{}]}]}]}}`)

	c := newTestClient(t, server)

	_, err := c.GetMediaPath(t.Context(), testPMS(t, server), "999999999")
	assert.ErrorIs(t, err, plex.ErrNoFilePathFound)
}

func TestIntegration_DiscoverServers(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, `<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer>
				<Device name="My Server" address="192.168.1.100" port="32400" accessToken="discovered-token">
					<Connection address="192.168.1.100" port="32400"/>
				</Device>
			</MediaContainer>`))

	servers, err := c.DiscoverServers(t.Context())
	require.NoError(t, err)
	require.Len(t, servers, 1)
	assert.Equal(t, "My Server", servers[0].Name)
	assert.Equal(t, "192.168.1.100", servers[0].Address)
	assert.Equal(t, 32400, servers[0].Port)
	assert.Equal(t, "discovered-token", servers[0].Token)
}

func TestIntegration_Ping(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, "")

	c := newTestClient(t, server)
	require.NoError(t, c.Ping(t.Context(), testPMS(t, server)))
}

func TestIntegration_Ping_Error(t *testing.T) {
	t.Parallel()

	server := newStatusServer(t, http.StatusInternalServerError, "error")

	c := newTestClient(t, server)
	assert.Error(t, c.Ping(t.Context(), testPMS(t, server)))
}

func TestIntegration_GetServerIdentity(t *testing.T) {
	t.Parallel()

	server := newTestServer(t,
		`{"MediaContainer":{"machineIdentifier":"abc123","version":"1.40.0"}}`)

	c := newTestClient(t, server)

	identity, err := c.GetServerIdentity(t.Context(), testPMS(t, server))
	require.NoError(t, err)
	assert.Equal(t, "abc123", identity.MachineIdentifier)
	assert.Equal(t, "1.40.0", identity.Version)
}

func TestIntegration_HTTPError(t *testing.T) {
	t.Parallel()

	deadURL := deadServerURL(t)

	c := plex.NewClient(plex.ClientConfig{
		Product:  "outtake",
		ClientID: "test-client",
		Token:    "test-token",
		Timeout:  5 * time.Second,
		BaseURL:  deadURL,
	})

	_, err := c.GetLibraries(t.Context(), testPMSForURL(t, deadURL))
	require.Error(t, err)
	assert.ErrorContains(t, err, "get libraries")
}

func TestIntegration_DecodeError(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, "invalid xml")

	c := newTestClient(t, server)

	_, err := c.GetLibraries(t.Context(), testPMS(t, server))
	require.Error(t, err)
	assert.ErrorContains(t, err, "decode libraries")
}

func TestIntegration_MapPlexType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"movie", "movie"},
		{"show", "show"},
		{"episode", "episode"},
		{"season", "season"},
		{"album", "album"},
		{"track", "track"},
		{"artist", "artist"},
		{"photo", "photo"},
		{"clip", "clip"},
		{"unknown", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, plex.MapPlexType(tt.input))
		})
	}
}

func TestIntegration_EmptyLibraries(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, `{"MediaContainer":{}}`)

	c := newTestClient(t, server)

	libs, err := c.GetLibraries(t.Context(), testPMS(t, server))
	require.NoError(t, err)
	assert.Empty(t, libs)
}

func TestIntegration_MediaWithNonMetadataKey(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, `{"MediaContainer":{"Metadata":[
			{"key":"/library/sections/1/title","title":"Movies","type":"directory"},
			{"ratingKey":"100","title":"Test Movie","duration":7200000,"type":"movie"}
		]}}`)

	c := newTestClient(t, server)

	items, err := c.GetMedia(t.Context(), testPMS(t, server), "1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Test Movie", items[0].Title)
}

func TestIntegration_MultipleConnections(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, `<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer>
				<Device name="My Server" address="192.168.1.100" port="32400" accessToken="token1">
					<Connection address="192.168.1.100" port="32400"/>
					<Connection address="10.0.0.1" port="32400"/>
				</Device>
			</MediaContainer>`))

	servers, err := c.DiscoverServers(t.Context())
	require.NoError(t, err)
	require.Len(t, servers, 2)
	assert.Equal(t, "192.168.1.100", servers[0].Address)
	assert.Equal(t, "10.0.0.1", servers[1].Address)
}

func TestIntegration_DeviceWithNoConnections(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, `<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer>
				<Device name="My Server" address="192.168.1.100" port="32400" accessToken="token1"/>
			</MediaContainer>`))

	servers, err := c.DiscoverServers(t.Context())
	require.NoError(t, err)
	assert.Empty(t, servers)
}

func TestIntegration_MultipleDevices(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, newTestServer(t, `<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer>
				<Device name="Server 1" address="192.168.1.100" port="32400" accessToken="token1">
					<Connection address="192.168.1.100" port="32400"/>
				</Device>
				<Device name="Server 2" address="192.168.1.200" port="32400" accessToken="token2">
					<Connection address="192.168.1.200" port="32400"/>
				</Device>
			</MediaContainer>`))

	servers, err := c.DiscoverServers(t.Context())
	require.NoError(t, err)
	require.Len(t, servers, 2)
	assert.Equal(t, "Server 1", servers[0].Name)
	assert.Equal(t, "Server 2", servers[1].Name)
}

func TestIntegration_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	server := newTestServer(t,
		`{"MediaContainer":{"Directory":[{"key":"1","title":"Movies","type":"movie"}]}}`)

	c := newTestClient(t, server)
	pms := testPMS(t, server)

	done := make(chan error, 3)

	for range 3 {
		go func() {
			_, err := c.GetLibraries(t.Context(), pms)
			done <- err
		}()
	}

	for range 3 {
		require.NoError(t, <-done)
	}
}
