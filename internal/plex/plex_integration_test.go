// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex_test

import (
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/mocks"
)

const (
	testProduct     = "outtake"
	testClientID    = "test-client"
	testToken       = "test-token"
	testBaseURL     = "http://localhost:32400"
	testServerName  = "Test"
	testServerHost  = "127.0.0.1"
	testHTTPScheme  = "http"
	testUnknownType = "unknown"
)

func newTestClient(t *testing.T, mockHTTP *mocks.MockHTTPClient) *plex.Client {
	t.Helper()

	return plex.NewClientWithHTTPClient(
		plex.ClientConfig{
			Product:  testProduct,
			ClientID: testClientID,
			Token:    testToken,
			Timeout:  0,
			BaseURL:  testBaseURL,
		},
		mockHTTP,
	)
}

func newResponse(statusCode int, body string) *client.Response {
	resp := client.AcquireResponse()
	resp.RawResponse.SetStatusCode(statusCode)
	resp.RawResponse.SetBodyString(body)

	return resp
}

func TestIntegration_NewClient(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, mocks.NewMockHTTPClient(t))
	assert.Equal(t, testProduct, c.Product)
	assert.Equal(t, testClientID, c.ClientID)
}

func TestIntegration_SetToken(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, mocks.NewMockHTTPClient(t))
	c.SetToken("new-token")
	assert.Equal(t, "new-token", c.Token)
}

func TestIntegration_SetBaseURL(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, mocks.NewMockHTTPClient(t))
	require.NoError(t, c.SetBaseURL("http://192.168.1.100:32400"))
}

func TestIntegration_SetBaseURL_Invalid(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, mocks.NewMockHTTPClient(t))
	assert.Error(t, c.SetBaseURL("://invalid"))
}

func TestIntegration_GeneratePIN(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Post(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"id": 12345, "code": "abc123"}`), nil).Once()

	c := newTestClient(t, mockHTTP)
	pin, err := c.GeneratePIN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 12345, pin.ID)
	assert.Equal(t, "abc123", pin.Code)
}

func TestIntegration_GeneratePIN_ServerError(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Post(mock.Anything, mock.Anything).
		Return(newResponse(500, "internal error"), nil).Once()

	c := newTestClient(t, mockHTTP)
	_, err := c.GeneratePIN(t.Context())
	assert.Error(t, err)
}

func TestIntegration_PollPIN(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"authToken": "token-xyz"}`), nil).Once()

	c := newTestClient(t, mockHTTP)
	token, err := c.PollPIN(t.Context(), 12345, "testpin")
	require.NoError(t, err)
	assert.Equal(t, "token-xyz", token)
}

func TestIntegration_PollPIN_NotClaimed(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"authToken": ""}`), nil).Once()

	c := newTestClient(t, mockHTTP)
	token, err := c.PollPIN(t.Context(), 12345, "testpin")
	require.ErrorIs(t, err, plex.ErrPINNotYetClaimed)
	assert.Empty(t, token)
}

func TestIntegration_ValidateToken(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"id": 1, "title": "Test User"}`), nil).Once()

	c := newTestClient(t, mockHTTP)
	valid, user, err := c.ValidateToken(t.Context())
	require.NoError(t, err)
	assert.True(t, valid)
	assert.Equal(t, "Test User", user.Title)
}

func TestIntegration_ValidateToken_Invalid(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(401, "unauthorized"), nil).Once()

	c := newTestClient(t, mockHTTP)
	valid, _, err := c.ValidateToken(t.Context())
	require.ErrorIs(t, err, plex.ErrUnauthorized)
	assert.False(t, valid)
}

func TestIntegration_GetAuthURL(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, mocks.NewMockHTTPClient(t))
	authURL := c.GetAuthURL("pin-code", "my-client", "http://localhost:8080/callback")
	assert.True(t, strings.HasPrefix(authURL, "https://app.plex.tv/auth#?"))
	assert.NotContains(t, authURL, "#%3F")
	assert.Contains(t, authURL, "clientID=my-client")
	assert.Contains(t, authURL, "code=pin-code")
}

func TestIntegration_GetLibraries(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"MediaContainer":{"Directory":[
			{"key":"1","title":"Movies","type":"movie"},
			{"key":"2","title":"TV Shows","type":"show"}
		]}}`), nil).Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}

	libs, err := c.GetLibraries(t.Context(), server)
	require.NoError(t, err)
	assert.Len(t, libs, 2)
	assert.Equal(t, "Movies", libs[0].Title)
	assert.Equal(t, "TV Shows", libs[1].Title)
}

func TestIntegration_GetMedia(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"MediaContainer":{"Metadata":[
			{"ratingKey":"100","title":"Test Movie","duration":7200000,"thumb":"/library/metadata/100/thumb/1","type":"movie"}
		]}}`), nil).Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}

	items, err := c.GetMedia(t.Context(), server, "1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.InEpsilon(t, 7200.0, items[0].Duration, 0.01)
	assert.Equal(t, "Test Movie", items[0].Title)
}

func TestIntegration_GetMediaPath(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"MediaContainer":{"Metadata":[
			{"Media":[{"Part":[{"file":"/media/movies/Test Movie.mkv"}]}]}
		]}}`), nil).Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}

	path, err := c.GetMediaPath(t.Context(), server, "100")
	require.NoError(t, err)
	assert.Equal(t, "/media/movies/Test Movie.mkv", path)
}

func TestIntegration_GetMediaPath_NotFound(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"MediaContainer":{"Metadata":[{"Media":[{"Part":[{}]}]}]}}`), nil).
		Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}

	_, err := c.GetMediaPath(t.Context(), server, "999999999")
	assert.ErrorIs(t, err, plex.ErrNoFilePathFound)
}

func TestIntegration_GetSessions(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer size="1">
				<Video title="Now Playing" duration="3600000">
					<Session id="sess-123"/>
				</Video>
			</MediaContainer>`), nil).Once()

	c := newTestClient(t, mockHTTP)

	sessions, err := c.GetSessions(t.Context())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.InEpsilon(t, 3600.0, sessions[0].Duration, 0.01)
	assert.Equal(t, "Now Playing", sessions[0].Title)
}

func TestIntegration_DiscoverServers(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer>
				<Device name="My Server" address="192.168.1.100" port="32400" accessToken="discovered-token">
					<Connection address="192.168.1.100" port="32400"/>
				</Device>
			</MediaContainer>`), nil).Once()

	c := newTestClient(t, mockHTTP)
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

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, ""), nil).Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}
	require.NoError(t, c.Ping(t.Context(), server))
}

func TestIntegration_Ping_Error(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(500, "error"), nil).Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}
	assert.Error(t, c.Ping(t.Context(), server))
}

func TestIntegration_GetServerIdentity(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"MediaContainer":{"machineIdentifier":"abc123","version":"1.40.0"}}`), nil).
		Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}

	identity, err := c.GetServerIdentity(t.Context(), server)
	require.NoError(t, err)
	assert.Equal(t, "abc123", identity.MachineIdentifier)
	assert.Equal(t, "1.40.0", identity.Version)
}

func TestIntegration_HTTPError(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(nil, assert.AnError).Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}
	_, err := c.GetLibraries(t.Context(), server)
	assert.Error(t, err)
}

func TestIntegration_DecodeError(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, "invalid xml"), nil).Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}
	_, err := c.GetLibraries(t.Context(), server)
	assert.Error(t, err)
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
		{testUnknownType, testUnknownType},
		{"", testUnknownType},
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

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"MediaContainer":{}}`), nil).
		Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}
	libs, err := c.GetLibraries(t.Context(), server)
	require.NoError(t, err)
	assert.Empty(t, libs)
}

func TestIntegration_MediaWithNonMetadataKey(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"MediaContainer":{"Metadata":[
			{"key":"/library/sections/1/title","title":"Movies","type":"directory"},
			{"ratingKey":"100","title":"Test Movie","duration":7200000,"type":"movie"}
		]}}`), nil).Once()

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}
	items, err := c.GetMedia(t.Context(), server, "1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Test Movie", items[0].Title)
}

func TestIntegration_MultipleConnections(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer>
				<Device name="My Server" address="192.168.1.100" port="32400" accessToken="token1">
					<Connection address="192.168.1.100" port="32400"/>
					<Connection address="10.0.0.1" port="32400"/>
				</Device>
			</MediaContainer>`), nil).Once()

	c := newTestClient(t, mockHTTP)
	servers, err := c.DiscoverServers(t.Context())
	require.NoError(t, err)
	assert.Len(t, servers, 2)
	assert.Equal(t, "192.168.1.100", servers[0].Address)
	assert.Equal(t, "10.0.0.1", servers[1].Address)
}

func TestIntegration_DeviceWithNoConnections(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer>
				<Device name="My Server" address="192.168.1.100" port="32400" accessToken="token1"/>
			</MediaContainer>`), nil).Once()

	c := newTestClient(t, mockHTTP)
	servers, err := c.DiscoverServers(t.Context())
	require.NoError(t, err)
	assert.Empty(t, servers)
}

func TestIntegration_MultipleDevices(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer>
				<Device name="Server 1" address="192.168.1.100" port="32400" accessToken="token1">
					<Connection address="192.168.1.100" port="32400"/>
				</Device>
				<Device name="Server 2" address="192.168.1.200" port="32400" accessToken="token2">
					<Connection address="192.168.1.200" port="32400"/>
				</Device>
			</MediaContainer>`), nil).Once()

	c := newTestClient(t, mockHTTP)
	servers, err := c.DiscoverServers(t.Context())
	require.NoError(t, err)
	assert.Len(t, servers, 2)
	assert.Equal(t, "Server 1", servers[0].Name)
	assert.Equal(t, "Server 2", servers[1].Name)
}

func TestIntegration_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	mockHTTP := mocks.NewMockHTTPClient(t)
	mockHTTP.EXPECT().Get(mock.Anything, mock.Anything).
		Return(newResponse(200, `{"MediaContainer":{"Directory":[{"key":"1","title":"Movies","type":"movie"}]}}`), nil).
		Times(3)

	c := newTestClient(t, mockHTTP)
	server := plex.Server{
		Name:    testServerName,
		Address: testServerHost,
		Port:    32400,
		Token:   testToken,
		Scheme:  testHTTPScheme,
		Local:   false,
	}

	done := make(chan error, 3)

	for range 3 {
		go func() {
			_, err := c.GetLibraries(t.Context(), server)
			done <- err
		}()
	}

	for range 3 {
		require.NoError(t, <-done)
	}
}
