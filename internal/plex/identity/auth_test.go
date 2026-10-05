// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity/mocks"
)

// plexRoute is a canned Plex API response.
type plexRoute struct {
	// status is the HTTP status the route answers with.
	status int

	// body is the response body the route answers with.
	body string
}

// testBoundServer is the connection bindServer selects out of testOneServerXML.
var testBoundServer = plex.Server{
	Name:    "Attic",
	Address: "192.168.1.9",
	Port:    32400,
	Token:   "discovered-token",
	Scheme:  "http",
	Local:   true,
}

// newPlexServer serves canned Plex API responses keyed by request path.
//
// Parameters:
//   - t: The test the server belongs to.
//   - routes: Canned responses, where an empty map answers everything with 404.
//
// Returns:
//   - server: The running Plex stand-in.
func newPlexServer(t *testing.T, routes map[string]plexRoute) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	for path, route := range routes {
		mux.HandleFunc(path, func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(route.status)

			if route.body != "" {
				_, _ = writer.Write([]byte(route.body))
			}
		})
	}

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

// testAuth builds an authentication service whose Plex calls reach baseURL.
//
// Parameters:
//   - t: The test the service belongs to.
//   - baseURL: Origin every Plex request is aimed at, which is empty to leave
//     the client on its own default.
//   - store: Token persistence, which may be nil.
//   - selected: Plex server binding, which may be nil.
//
// Returns:
//   - auth: The authentication service under test.
func testAuth(
	t *testing.T,
	baseURL string,
	store TokenStore,
	selected ServerBinding,
) *Auth {
	t.Helper()

	auth := New("outtake", "test-client", "http://localhost:8080", store, selected)

	if baseURL != "" {
		auth.newClient = func(cfg plex.ClientConfig) *plex.Client {
			cfg.BaseURL = baseURL

			return plex.NewClient(cfg)
		}
	}

	return auth
}

// userRoute is the plex.tv answer that validates an access token.
func userRoute() plexRoute {
	return plexRoute{status: http.StatusOK, body: `{"id": 42, "title": "Nick"}`}
}

func TestBoundWithoutABinding(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, nil)

	assert.False(t, auth.Bound())
}

func TestBoundReportsTheSelection(t *testing.T) {
	t.Parallel()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.Server{Name: "Attic"}, true).
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, bound)

	assert.True(t, auth.Bound())
}

func TestBoundWithoutASelection(t *testing.T) {
	t.Parallel()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.EmptyServer(), false).
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, bound)

	assert.False(t, auth.Bound())
}

func TestClientWithoutABinding(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, nil)

	client, server, ok := auth.Client()
	assert.Nil(t, client)
	assert.Empty(t, server.Name)
	assert.False(t, ok)
}

func TestClientFromTheBinding(t *testing.T) {
	t.Parallel()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Client().
		Return(nil, plex.Server{Name: "Attic", Token: "access-token"}, true).
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, bound)

	client, server, ok := auth.Client()
	require.True(t, ok)
	assert.Nil(t, client)
	assert.Equal(t, "Attic", server.Name)
	assert.Equal(t, "access-token", server.Token)
}

func TestSelectedWithoutABinding(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, nil)

	server, ok := auth.Selected()
	assert.Empty(t, server.Name)
	assert.False(t, ok)
}

func TestSelectedFromTheBinding(t *testing.T) {
	t.Parallel()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.Server{Name: "Attic"}, true).
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, bound)

	server, ok := auth.Selected()
	require.True(t, ok)
	assert.Equal(t, "Attic", server.Name)
}

func TestSessionsWithoutABinding(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, nil)

	assert.Nil(t, auth.Sessions())
}

func TestSessionsFromTheBinding(t *testing.T) {
	t.Parallel()

	sessions := []plex.Session{{ID: "sess-1", Title: "Now Playing"}}

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Sessions().
		Return(sessions).
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, bound)

	assert.Equal(t, sessions, auth.Sessions())
}

func TestRestoreWithoutAStore(t *testing.T) {
	t.Parallel()

	assert.Empty(t, Restore(t.Context(), nil))
}

func TestRestoreReportsAFailedRead(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		LatestToken(mock.Anything).
		Return("", errStoreClosed).
		Once()

	assert.Empty(t, Restore(t.Context(), store))
}

func TestRestoreReturnsThePersistedToken(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		LatestToken(mock.Anything).
		Return("access-token", nil).
		Once()

	assert.Equal(t, "access-token", Restore(t.Context(), store))
}

func TestSaveTokenWithoutAStore(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, nil)

	require.NoError(t, auth.saveToken(t.Context(), "access-token"))
}

func TestSaveTokenPersistsForTheServiceIdentity(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveToken(mock.Anything, "test-client", "access-token").
		Return(nil).
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", store, nil)

	require.NoError(t, auth.saveToken(t.Context(), "access-token"))
}

func TestSaveTokenReportsAFailedWrite(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveToken(mock.Anything, "test-client", "access-token").
		Return(errStoreClosed).
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", store, nil)

	err := auth.saveToken(t.Context(), "access-token")
	require.ErrorIs(t, err, errStoreClosed)
	require.ErrorContains(t, err, "save token")
}

func TestValidateReturnsTheUserBehindTheToken(t *testing.T) {
	t.Parallel()

	server := newPlexServer(t, map[string]plexRoute{"/api/v2/user": userRoute()})
	auth := testAuth(t, server.URL, nil, nil)

	assert.Equal(t, 42, auth.validate(t.Context(), "access-token"))
}

func TestValidateRejectsAnUnauthorizedToken(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/v2/user": {status: http.StatusUnauthorized, body: "unauthorized"},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	assert.Zero(t, auth.validate(t.Context(), "access-token"))
}

func TestValidateReportsARequestFailure(t *testing.T) {
	t.Parallel()

	auth := testAuth(t, newPlexServer(t, nil).URL, nil, nil)

	assert.Zero(t, auth.validate(t.Context(), "access-token"))
}

func TestClientForPresentsTheServiceIdentity(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, nil)

	client := auth.clientFor("access-token")

	require.NotNil(t, client)
	assert.Equal(t, "outtake", client.Product)
	assert.Equal(t, "test-client", client.ClientID)
	assert.Equal(t, "access-token", client.Token)
}

func TestClientForPresentsAnUnauthenticatedClient(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, nil)

	assert.Empty(t, auth.clientFor("").Token)
}

func TestBeginPINReturnsTheAuthorizationURL(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/v2/pins": {status: http.StatusOK, body: `{"id": 4321, "code": "ABCD"}`},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	pin, err := auth.BeginPIN(t.Context())
	require.NoError(t, err)

	assert.Equal(t, 4321, pin.ID)
	assert.Equal(t, "ABCD", pin.Code)
	assert.Contains(t, pin.URL, "clientID="+"test-client")
	assert.Contains(t, pin.URL, "code=ABCD")
	assert.Contains(t, pin.URL, "forwardUrl="+url.QueryEscape("http://localhost:8080"+forwardPath))
}

func TestBeginPINReportsAFailedRequest(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/v2/pins": {status: http.StatusInternalServerError, body: "boom"},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	_, err := auth.BeginPIN(t.Context())
	require.ErrorContains(t, err, "generate pin")
}

func TestPollPINReturnsTheTokenOnceClaimed(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/v2/pins/4321": {status: http.StatusOK, body: `{"authToken": "claimed-token"}`},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	accessToken, err := auth.PollPIN(t.Context(), 4321, "ABCD")
	require.NoError(t, err)
	assert.Equal(t, "claimed-token", accessToken)
}

func TestPollPINReportsAnUnclaimedPIN(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/v2/pins/4321": {status: http.StatusOK, body: `{"authToken": ""}`},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	accessToken, err := auth.PollPIN(t.Context(), 4321, "ABCD")
	require.ErrorIs(t, err, plex.ErrPINNotYetClaimed)
	assert.Empty(t, accessToken)
}

func TestPollPINReportsAFailedRequest(t *testing.T) {
	t.Parallel()

	auth := testAuth(t, newPlexServer(t, nil).URL, nil, nil)

	_, err := auth.PollPIN(t.Context(), 4321, "ABCD")
	require.ErrorContains(t, err, "poll pin")
}

func TestDiscoverReturnsEveryConnection(t *testing.T) {
	t.Parallel()

	const testOneServerXML = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
	<Device name="Attic" address="10.0.0.5" port="32400" accessToken="discovered-token">
		<Connection protocol="https" address="10.0.0.5" port="32400"/>
		<Connection protocol="http" address="192.168.1.9" port="32400" local="1"/>
	</Device>
</MediaContainer>`

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: testOneServerXML},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	servers, err := auth.Discover(t.Context(), "access-token")
	require.NoError(t, err)

	require.Len(t, servers, 2, "both connections of the one device are reported")
	assert.Equal(t, "Attic", servers[0].Name)
	assert.False(t, servers[0].Local)
	assert.Equal(t, testBoundServer, servers[1])
}

func TestDiscoverReportsAFailedRequest(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: "not xml at all"},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	_, err := auth.Discover(t.Context(), "access-token")
	require.ErrorContains(t, err, "discover servers")
}

func TestLoginAcceptsAValidToken(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveToken(mock.Anything, "test-client", "access-token").
		Return(nil).
		Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}
	auth := testAuth(t, newPlexServer(t, routes).URL, store, nil)

	userID, err := auth.Login(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, 42, userID)
}

func TestLoginRejectsATokenPlexRefuses(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)

	routes := map[string]plexRoute{
		"/api/v2/user": {status: http.StatusUnauthorized, body: "unauthorized"},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, store, nil)

	userID, err := auth.Login(t.Context(), "access-token")
	require.ErrorIs(t, err, ErrInvalidToken)
	assert.Zero(t, userID, "a refused token never reaches the store")
}

func TestLoginReportsAFailedWrite(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveToken(mock.Anything, "test-client", "access-token").
		Return(errStoreClosed).
		Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}
	auth := testAuth(t, newPlexServer(t, routes).URL, store, nil)

	userID, err := auth.Login(t.Context(), "access-token")
	require.ErrorIs(t, err, errStoreClosed)
	require.ErrorContains(t, err, "login")
	assert.Equal(t, 42, userID, "the user behind the token is still reported")
}

func TestLogoutClearsTheSelectionAndCredentials(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		ClearAuth(mock.Anything).
		Return(nil).
		Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Clear().
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", store, bound)

	require.NoError(t, auth.Logout(t.Context()))
}

func TestLogoutReportsAFailedClear(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		ClearAuth(mock.Anything).
		Return(errStoreClosed).
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", store, nil)

	err := auth.Logout(t.Context())
	require.ErrorIs(t, err, errStoreClosed)
	require.ErrorContains(t, err, "clear auth")
}

func TestLogoutToleratesAbsentCollaborators(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, nil)

	require.NoError(t, auth.Logout(t.Context()))
}

func TestAcceptReturnsTheUserBehindTheToken(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveToken(mock.Anything, "test-client", "access-token").
		Return(nil).
		Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}
	auth := testAuth(t, newPlexServer(t, routes).URL, store, nil)

	userID, err := auth.Accept(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, 42, userID)
}

func TestAcceptReportsAFailedWrite(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveToken(mock.Anything, "test-client", "access-token").
		Return(errStoreClosed).
		Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}
	auth := testAuth(t, newPlexServer(t, routes).URL, store, nil)

	userID, err := auth.Accept(t.Context(), "access-token")
	require.ErrorIs(t, err, errStoreClosed)
	require.ErrorContains(t, err, "accept token")
	assert.Equal(t, 42, userID)
}

func TestBindServerBindsTheOnlyDiscoveredServer(t *testing.T) {
	t.Parallel()

	const testOneServerXML = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
	<Device name="Attic" address="10.0.0.5" port="32400" accessToken="discovered-token">
		<Connection protocol="https" address="10.0.0.5" port="32400"/>
		<Connection protocol="http" address="192.168.1.9" port="32400" local="1"/>
	</Device>
</MediaContainer>`

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveSelectedServer(mock.Anything, testBoundServer).
		Return(nil).
		Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.EmptyServer(), false).
		Once()
	bound.EXPECT().
		Set(testBoundServer).
		Once()

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: testOneServerXML},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, store, bound)
	auth.bindServer(t.Context(), "access-token")
}

func TestBindServerReportsAFailedDiscovery(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.EmptyServer(), false).
		Once()

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: "not xml at all"},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, store, bound)
	auth.bindServer(t.Context(), "access-token")
}

func TestBindServerSkipsAnEmptyDiscovery(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.EmptyServer(), false).
		Once()

	routes := map[string]plexRoute{
		"/api/resources": {
			status: http.StatusOK,
			body:   `<?xml version="1.0" encoding="UTF-8"?><MediaContainer/>`,
		},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, store, bound)
	auth.bindServer(t.Context(), "access-token")
}

func TestBindServerSkipsSeveralDiscoveredServers(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.EmptyServer(), false).
		Once()

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
	<Device name="Attic" address="10.0.0.5" port="32400" accessToken="discovered-token">
		<Connection protocol="http" address="192.168.1.9" port="32400" local="1"/>
	</Device>
	<Device name="Cellar" address="10.0.0.6" port="32400" accessToken="other-token">
		<Connection protocol="http" address="192.168.1.10" port="32400"/>
	</Device>
</MediaContainer>`},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, store, bound)
	auth.bindServer(t.Context(), "access-token")
}

func TestBindServerSkipsAnExistingSelection(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.Server{Name: "Attic"}, true).
		Once()

	auth := testAuth(t, "", store, bound)
	auth.bindServer(t.Context(), "access-token")
}

func TestBindServerSkipsWithoutABinding(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockTokenStore(t)

	auth := testAuth(t, "", store, nil)
	auth.bindServer(t.Context(), "access-token")
}

func TestBindServerSkipsWithoutAStore(t *testing.T) {
	t.Parallel()

	bound := mocks.NewMockServerBinding(t)

	auth := testAuth(t, "", nil, bound)
	auth.bindServer(t.Context(), "access-token")
}

func TestBindServerReportsAFailedPersist(t *testing.T) {
	t.Parallel()

	const testOneServerXML = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
	<Device name="Attic" address="10.0.0.5" port="32400" accessToken="discovered-token">
		<Connection protocol="https" address="10.0.0.5" port="32400"/>
		<Connection protocol="http" address="192.168.1.9" port="32400" local="1"/>
	</Device>
</MediaContainer>`

	store := mocks.NewMockTokenStore(t)
	store.EXPECT().
		SaveSelectedServer(mock.Anything, testBoundServer).
		Return(errStoreClosed).
		Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.EmptyServer(), false).
		Once()
	bound.EXPECT().
		Set(testBoundServer).
		Once()

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: testOneServerXML},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, store, bound)
	auth.bindServer(t.Context(), "access-token")
}
