// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

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

// testOneServerXML is a plex.tv resources answer listing one server with a
// remote and a local connection.
const testOneServerXML = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
	<Device name="Attic" owned="1" address="10.0.0.5" port="32400" accessToken="discovered-token">
		<Connection protocol="https" address="10.0.0.5" port="32400"/>
		<Connection protocol="http" address="192.168.1.9" port="32400" local="1"/>
	</Device>
</MediaContainer>`

// testSharedServerXML is a plex.tv resources answer listing a server the
// account owns and a server shared with it.
const testSharedServerXML = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
	<Device name="Attic" owned="1" clientIdentifier="machine-1" provides="server" accessToken="owner-token">
		<Connection protocol="http" address="192.168.1.9" port="32400" local="1"/>
	</Device>
	<Device name="Friend" owned="0" clientIdentifier="machine-2" provides="server" accessToken="shared-token">
		<Connection protocol="http" address="192.168.1.10" port="32400" local="1"/>
	</Device>
</MediaContainer>`

// testBoundServer is the connection bindServer selects out of testOneServerXML.
var testBoundServer = plex.Server{
	Name:    "Attic",
	Address: "192.168.1.9",
	Port:    32400,
	Token:   "discovered-token",
	Scheme:  "http",
	Local:   true,
	Owned:   true,
}

// testFriendServer is the server testSharedServerXML lists as shared, bound
// with its owner's token.
var testFriendServer = plex.Server{
	Name:      "Friend",
	Address:   "192.168.1.10",
	Port:      32400,
	Token:     "friend-owner-token",
	Scheme:    "http",
	Local:     true,
	MachineID: "machine-2",
	Relay:     false,
	Owned:     false,
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
//   - store: Owner and server persistence, which may be nil.
//   - selected: Plex server binding, which may be nil.
//
// Returns:
//   - auth: The authentication service under test.
func testAuth(
	t *testing.T,
	baseURL string,
	store Store,
	selected ServerBinding,
) *Auth {
	t.Helper()

	if baseURL == "" {
		return New("outtake", "test-client", "http://localhost:8080", store, selected)
	}

	return New(
		"outtake", "test-client", "http://localhost:8080", store, selected,
		WithPlexURL(baseURL),
	)
}

// newAccountServer stands in for plex.tv, answering the user endpoint with the
// account each token belongs to and 401 for any other token.
//
// Parameters:
//   - t: The test the server belongs to.
//   - accounts: The plex.tv user JSON per access token.
//
// Returns:
//   - server: The running plex.tv stand-in.
func newAccountServer(t *testing.T, accounts map[string]string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/user", func(writer http.ResponseWriter, request *http.Request) {
		account, ok := accounts[request.Header.Get("X-Plex-Token")]
		if !ok {
			writer.WriteHeader(http.StatusUnauthorized)

			return
		}

		_, _ = writer.Write([]byte(account))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

// reachablePlexTV stands in for plex.tv and for the one server it lists.
//
// The server's remote connection points at a dead port and its local connection
// at the stand-in itself, which answers /identity as machine-1.
//
// Parameters:
//   - t: The test the stand-in belongs to.
//
// Returns:
//   - origin: Where Plex requests are aimed.
//   - bound: The connection binding should pick.
//   - dead: The listed connection that does not answer.
func reachablePlexTV(t *testing.T) (string, plex.Server, plex.Server) {
	t.Helper()

	closedPort := deadPort(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/user", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(userRoute().body))
	})
	mux.HandleFunc("/identity", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"MediaContainer":{"machineIdentifier":"machine-1"}}`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	addr, ok := server.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	port := addr.Port

	mux.HandleFunc("/api/resources", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(writer, `<MediaContainer>
	<Device name="Attic" owned="1" clientIdentifier="machine-1" provides="server" accessToken="discovered-token">
		<Connection protocol="https" address="127.0.0.1" port="%d" local="0"/>
		<Connection protocol="http" address="127.0.0.1" port="%d" local="1"/>
	</Device>
</MediaContainer>`, closedPort, port)
	})

	bound := plex.Server{
		Name:      "Attic",
		Address:   "127.0.0.1",
		Port:      port,
		Token:     "discovered-token",
		Scheme:    "http",
		Local:     true,
		MachineID: "machine-1",
		Relay:     false,
		Owned:     true,
	}

	dead := bound

	dead.Port = closedPort
	dead.Scheme = "https"
	dead.Local = false

	return server.URL, bound, dead
}

// deadPort returns the loopback port of a test server that answers nothing.
//
// The server closes each request's connection unanswered and holds its port
// until the test ends, so no other listener can answer in its place.
//
// Parameters:
//   - t: The test that needs the port.
//
// Returns:
//   - port: The server's port.
func deadPort(t *testing.T) int {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(dropRequest))
	t.Cleanup(server.Close)

	addr, ok := server.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "a test server listens on TCP")

	return addr.Port
}

// dropRequest closes a request's connection without answering it.
//
// Parameters:
//   - writer: The response, whose connection is taken and closed.
func dropRequest(writer http.ResponseWriter, _ *http.Request) {
	conn, _, err := http.NewResponseController(writer).Hijack()
	if err == nil {
		_ = conn.Close()
	}
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

// TestClientAsFollowsTheViewer covers the client each viewer reads the bound
// server with.
//
// The owner gets the bound client, and anyone else a client carrying their
// own token.
func TestClientAsFollowsTheViewer(t *testing.T) {
	t.Parallel()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Client().
		Return(nil, plex.Server{Name: "Attic", Token: "owner-token"}, true).
		Once()
	bound.EXPECT().
		ClientWithToken("member-token").
		Return(nil, plex.Server{Name: "Attic", Token: "member-token"}, true).
		Once()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, bound)

	_, server, ok := auth.ClientAs(OwnerViewer(42))
	require.True(t, ok)
	assert.Equal(t, "owner-token", server.Token)

	_, server, ok = auth.ClientAs(Viewer{Token: "member-token", PlexID: 7, Owner: false})
	require.True(t, ok)
	assert.Equal(t, "member-token", server.Token)

	_, _, ok = New("outtake", "test-client", "http://localhost:8080", nil, nil).
		ClientAs(Viewer{Token: "member-token", PlexID: 7, Owner: false})
	assert.False(t, ok, "nothing is read without a binding")
}

// TestServerTokenFindsTheAccountsTokenForTheBoundServer covers the token a
// member reads the bound server with.
//
// It is the one the account's own discovery lists for the bound machine,
// shared or owned, and there is none for an account that cannot reach it.
func TestServerTokenFindsTheAccountsTokenForTheBoundServer(t *testing.T) {
	t.Parallel()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(testFriendServer, true).Once()

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: testSharedServerXML},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, bound)

	token, err := auth.ServerToken(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, "shared-token", token, "a server shared with the account")

	unknown := testFriendServer

	unknown.MachineID = "machine-9"

	elsewhere := mocks.NewMockServerBinding(t)
	elsewhere.EXPECT().Get().Return(unknown, true).Once()

	token, err = testAuth(t, newPlexServer(t, routes).URL, nil, elsewhere).
		ServerToken(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Empty(t, token, "a server not shared with the account")

	failing := mocks.NewMockServerBinding(t)
	failing.EXPECT().Get().Return(testFriendServer, true).Once()

	down := map[string]plexRoute{
		"/api/resources": {status: http.StatusServiceUnavailable, body: ""},
	}

	_, err = testAuth(
		t,
		newPlexServer(t, down).URL,
		nil,
		failing,
	).ServerToken(t.Context(), "access-token")
	require.Error(t, err, "plex.tv that cannot be asked is not an unshared server")
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

// TestLiveSessionsAsksTheBoundServer covers the fresh session read.
//
// It goes to the selected server itself, and refuses without one.
func TestLiveSessionsAsksTheBoundServer(t *testing.T) {
	t.Parallel()

	pms := newPlexServer(t, map[string]plexRoute{
		"/status/sessions": {
			status: http.StatusOK,
			body: `{"MediaContainer":{"Metadata":[{"ratingKey":"42","viewOffset":61500,` +
				`"Session":{"id":"s1"},"Player":{"state":"paused"}}]}}`,
		},
	})

	address, err := url.Parse(pms.URL)
	require.NoError(t, err)

	port, err := strconv.Atoi(address.Port())
	require.NoError(t, err)

	server := plex.Server{
		Name:    "Attic",
		Address: address.Hostname(),
		Port:    port,
		Token:   "server-token",
		Scheme:  "http",
		Local:   true,
	}

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Client().Return(plex.NewClient(plex.ClientConfig{
		Product:  "outtake",
		ClientID: "test-client",
		Token:    "server-token",
		Timeout:  time.Second,
		BaseURL:  "",
	}), server, true)

	auth := New("outtake", "test-client", "http://localhost:8080", nil, bound)

	sessions, err := auth.LiveSessions(t.Context())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "42", sessions[0].MediaItem.ID)
	assert.InDelta(t, 61.5, sessions[0].ViewOffset, 0.001)
	assert.Equal(t, plex.StatePaused, sessions[0].State)

	_, err = New("outtake", "test-client", "http://localhost:8080", nil, nil).
		LiveSessions(t.Context())
	assert.ErrorIs(t, err, ErrNoServer)
}

func TestValidateReturnsTheUserBehindTheToken(t *testing.T) {
	t.Parallel()

	server := newPlexServer(t, map[string]plexRoute{"/api/v2/user": userRoute()})
	auth := testAuth(t, server.URL, nil, nil)

	account, ok := auth.validate(t.Context(), "access-token")

	require.True(t, ok)
	assert.Equal(t, User{PlexID: 42, Username: "Nick", Role: ""}, account,
		"the title stands in for a missing username, and validation assigns no role")
}

func TestValidatePrefersTheUsernameOverTheTitle(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/v2/user": {
			status: http.StatusOK,
			body:   `{"id": 42, "username": "nick", "title": "Nick F"}`,
		},
	}

	account, ok := testAuth(
		t,
		newPlexServer(t, routes).URL,
		nil,
		nil,
	).validate(t.Context(), "access-token")

	require.True(t, ok)
	assert.Equal(t, "nick", account.Username)
}

func TestValidateRejectsAnUnauthorizedToken(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/v2/user": {status: http.StatusUnauthorized, body: "unauthorized"},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	_, ok := auth.validate(t.Context(), "access-token")
	assert.False(t, ok)
}

func TestValidateRejectsAnAccountWithoutAnID(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/v2/user": {status: http.StatusOK, body: `{"title": "Nobody"}`},
	}

	_, ok := testAuth(
		t,
		newPlexServer(t, routes).URL,
		nil,
		nil,
	).validate(t.Context(), "access-token")
	assert.False(t, ok)
}

func TestValidateReportsARequestFailure(t *testing.T) {
	t.Parallel()

	auth := testAuth(t, newPlexServer(t, nil).URL, nil, nil)

	_, ok := auth.validate(t.Context(), "access-token")
	assert.False(t, ok)
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
		"/api/v2/pins": {
			status: http.StatusOK,
			body:   `{"id": 4321, "code": "ABCD", "expiresIn": 1800}`,
		},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	pin, err := auth.BeginPIN(t.Context())
	require.NoError(t, err)

	assert.Equal(t, 30*time.Minute, pin.ExpiresIn, "the page stops polling when the PIN lapses")
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

// TestDiscoverLeavesOutAServerSharedWithTheAccount covers discovery for the
// servers page and the server choice.
//
// Only a server the account owns may be bound, so a shared one is not listed.
func TestDiscoverLeavesOutAServerSharedWithTheAccount(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: testSharedServerXML},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, nil, nil)

	servers, err := auth.Discover(t.Context(), "access-token")
	require.NoError(t, err)

	require.Len(t, servers, 1)
	assert.Equal(t, "machine-1", servers[0].MachineID)
	assert.True(t, servers[0].Owned)
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

func TestSignInClaimsAnUnownedInstallation(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("", nil).Once()
	store.EXPECT().ClaimOwner(mock.Anything, 42, "Nick").Return(true, nil).Once()
	store.EXPECT().ClearLegacyTokens(mock.Anything).Return(nil).Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}
	auth := testAuth(t, newPlexServer(t, routes).URL, store, nil)

	var heard []int

	auth.SetClaimedFunc(func(plexUserID int) { heard = append(heard, plexUserID) })

	user, err := auth.SignIn(t.Context(), "access-token")

	require.NoError(t, err)
	assert.Equal(t, User{PlexID: 42, Username: "Nick", Role: RoleOwner}, user)
	assert.Equal(t, []int{42}, heard, "the claim is announced once, with the owner")
}

// TestSignInRefusesAClaimOnABoundServerTheAccountDoesNotOwn covers an
// installation bound to a server before anyone owns it.
//
// The bound server is only shared with the account, so the account may not
// claim the installation.
func TestSignInRefusesAClaimOnABoundServerTheAccountDoesNotOwn(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("", nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(testFriendServer, true).Once()

	routes := map[string]plexRoute{
		"/api/v2/user":   userRoute(),
		"/api/resources": {status: http.StatusOK, body: testSharedServerXML},
	}

	_, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		bound,
	).SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrNotAllowed)
}

// TestSignInRefusesAClaimPlexCannotConfirm covers an installation bound to a
// server before anyone owns it, while plex.tv cannot list the account's
// servers.
//
// The claim is refused as one to try again, not as another account's.
func TestSignInRefusesAClaimPlexCannotConfirm(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("", nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(testFriendServer, true).Once()

	routes := map[string]plexRoute{
		"/api/v2/user":   userRoute(),
		"/api/resources": {status: http.StatusServiceUnavailable, body: ""},
	}

	_, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		bound,
	).SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrOwnershipUnverified)
	assert.NotErrorIs(t, err, ErrNotAllowed)
}

// TestSignInLetsTheBoundServersOwnerClaim covers an installation bound to a
// server before anyone owns it, claimed by the server's owner.
func TestSignInLetsTheBoundServersOwnerClaim(t *testing.T) {
	t.Parallel()

	owned := testFriendServer

	owned.Name = "Attic"
	owned.Address = "192.168.1.9"
	owned.Token = "owner-token"
	owned.MachineID = "machine-1"

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("", nil).Once()
	store.EXPECT().ClaimOwner(mock.Anything, 42, "Nick").Return(true, nil).Once()
	store.EXPECT().ClearLegacyTokens(mock.Anything).Return(nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(owned, true).Twice()

	routes := map[string]plexRoute{
		"/api/v2/user":   userRoute(),
		"/api/resources": {status: http.StatusOK, body: testSharedServerXML},
	}

	user, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		bound,
	).SignIn(t.Context(), "access-token")

	require.NoError(t, err)
	assert.Equal(t, RoleOwner, user.Role)
}

func TestSignInKeepsTheRoleOfAKnownUser(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("admin", true, nil).Once()
	store.EXPECT().TouchLogin(mock.Anything, 42, "Nick").Return(nil).Once()

	// The admin is shared the bound server, machine-2, and owns another.
	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(testFriendServer, true).Twice()

	routes := map[string]plexRoute{
		"/api/v2/user":   userRoute(),
		"/api/resources": {status: http.StatusOK, body: testSharedServerXML},
	}
	auth := testAuth(t, newPlexServer(t, routes).URL, store, bound)

	user, err := auth.SignIn(t.Context(), "access-token")

	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, user.Role)
}

func TestSignInStillSignsInWhenTheLoginCannotBeRecorded(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("owner", true, nil).Once()
	store.EXPECT().TouchLogin(mock.Anything, 42, "Nick").Return(errStoreClosed).Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}

	user, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		nil,
	).SignIn(t.Context(), "access-token")

	require.NoError(t, err)
	assert.Equal(t, RoleOwner, user.Role)
}

func TestSignInRefusesAnotherAccountOnceOwned(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(true, nil).Once()

	// The bound server is shared with neither of the account's servers, and
	// the binding answers only Get, so binding anything fails the test.
	elsewhere := testFriendServer

	elsewhere.MachineID = "machine-9"

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(elsewhere, true).Once()

	routes := map[string]plexRoute{
		"/api/v2/user":   userRoute(),
		"/api/resources": {status: http.StatusOK, body: testSharedServerXML},
	}
	auth := testAuth(t, newPlexServer(t, routes).URL, store, bound)

	_, err := auth.SignIn(t.Context(), "access-token")

	require.ErrorIs(
		t,
		err,
		ErrNotAllowed,
		"nothing is stored for an account the server is not shared with",
	)
}

// TestSignInAdmitsASharedAccountAsPending covers an account the owned
// installation's server is shared with, signing in for the first time.
//
// It is stored as pending and signs in as pending, which lets it only wait
// for an admin.
func TestSignInAdmitsASharedAccountAsPending(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(true, nil).Once()
	store.EXPECT().AddPendingUser(mock.Anything, 42, "Nick").Return(nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(testFriendServer, true).Twice()

	routes := map[string]plexRoute{
		"/api/v2/user":   userRoute(),
		"/api/resources": {status: http.StatusOK, body: testSharedServerXML},
	}

	user, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		bound,
	).SignIn(t.Context(), "access-token")

	require.NoError(t, err)
	assert.Equal(t, User{PlexID: 42, Username: "Nick", Role: RolePending}, user)
}

// TestSignInRefusesAMemberWhoseShareWasRemoved covers a stored member whose
// share of the bound server is gone, refused at sign-in.
func TestSignInRefusesAMemberWhoseShareWasRemoved(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("member", true, nil).Once()

	elsewhere := testFriendServer

	elsewhere.MachineID = "machine-9"

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(elsewhere, true).Once()

	routes := map[string]plexRoute{
		"/api/v2/user":   userRoute(),
		"/api/resources": {status: http.StatusOK, body: testSharedServerXML},
	}

	_, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		bound,
	).SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrNotAllowed)
}

// TestSignInRefusesAMemberPlexCannotVouchFor covers plex.tv failing while a
// member signs in, refused as one to try again rather than as another
// account's.
func TestSignInRefusesAMemberPlexCannotVouchFor(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("member", true, nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(testFriendServer, true).Once()

	routes := map[string]plexRoute{
		"/api/v2/user":   userRoute(),
		"/api/resources": {status: http.StatusServiceUnavailable, body: ""},
	}

	_, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		bound,
	).SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrAccessUnverified)
	assert.NotErrorIs(t, err, ErrNotAllowed)
}

func TestSignInRefusesAnInvalidTokenWithoutTouchingTheStore(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	bound := mocks.NewMockServerBinding(t)

	routes := map[string]plexRoute{
		"/api/v2/user": {status: http.StatusUnauthorized, body: "unauthorized"},
	}

	_, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		bound,
	).SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestSignInRefusesEveryAccountWithoutAStore(t *testing.T) {
	t.Parallel()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}

	_, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		nil,
		nil,
	).SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrNotAllowed)
}

func TestSignInLetsTheLegacyAccountClaimTheInstallation(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("legacy-token", nil).Once()
	store.EXPECT().ClaimOwner(mock.Anything, 42, "nick").Return(true, nil).Once()
	store.EXPECT().ClearLegacyTokens(mock.Anything).Return(nil).Once()

	plexTV := newAccountServer(t, map[string]string{
		"access-token": `{"id": 42, "username": "nick"}`,
		"legacy-token": `{"id": 42, "username": "nick"}`,
	})

	user, err := testAuth(t, plexTV.URL, store, nil).SignIn(t.Context(), "access-token")

	require.NoError(t, err)
	assert.Equal(t, RoleOwner, user.Role)
}

func TestSignInRefusesAnAccountTheLegacyTokenDoesNotName(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("legacy-token", nil).Once()

	plexTV := newAccountServer(t, map[string]string{
		"access-token": `{"id": 42, "username": "intruder"}`,
		"legacy-token": `{"id": 7, "username": "owner"}`,
	})

	_, err := testAuth(t, plexTV.URL, store, nil).SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrNotAllowed)
}

func TestSignInClaimsWhenTheLegacyTokenNoLongerValidates(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("revoked-token", nil).Once()
	store.EXPECT().ClaimOwner(mock.Anything, 42, "nick").Return(true, nil).Once()
	store.EXPECT().ClearLegacyTokens(mock.Anything).Return(nil).Once()

	plexTV := newAccountServer(t, map[string]string{
		"access-token": `{"id": 42, "username": "nick"}`,
	})

	user, err := testAuth(t, plexTV.URL, store, nil).SignIn(t.Context(), "access-token")

	require.NoError(t, err)
	assert.Equal(t, RoleOwner, user.Role)
}

func TestSignInRefusesWhenTheLegacyTokenCannotBeResolved(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("legacy-token", nil).Once()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/user", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Plex-Token") == "legacy-token" {
			writer.WriteHeader(http.StatusInternalServerError)

			return
		}

		_, _ = writer.Write([]byte(`{"id": 42, "username": "nick"}`))
	})

	plexTV := httptest.NewServer(mux)
	t.Cleanup(plexTV.Close)

	_, err := testAuth(t, plexTV.URL, store, nil).SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrNotAllowed,
		"a stored token Plex could not check still reserves the installation")
}

func TestSignInRefusesWhenTheLegacyTokenCannotBeRead(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("", errStoreClosed).Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}

	_, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		nil,
	).SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrNotAllowed)
}

func TestSignInAcceptsTheSameAccountWinningAConcurrentClaim(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("", nil).Once()
	store.EXPECT().ClaimOwner(mock.Anything, 42, "Nick").Return(false, nil).Once()
	store.EXPECT().UserRole(mock.Anything, 42).Return("owner", true, nil).Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}

	user, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		nil,
	).SignIn(t.Context(), "access-token")

	require.NoError(t, err)
	assert.Equal(t, RoleOwner, user.Role, "the other browser's claim made this account the owner")
}

func TestSignInRefusesWhenAnotherAccountWonAConcurrentClaim(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Twice()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("", nil).Once()
	store.EXPECT().ClaimOwner(mock.Anything, 42, "Nick").Return(false, nil).Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}

	auth := testAuth(t, newPlexServer(t, routes).URL, store, nil)

	auth.SetClaimedFunc(func(int) { t.Error("a lost claim is not announced") })

	_, err := auth.SignIn(t.Context(), "access-token")

	require.ErrorIs(t, err, ErrNotAllowed)
}

func TestSignInStillClaimsWhenTheLegacyTokensCannotBeCleared(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
	store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
	store.EXPECT().LegacyToken(mock.Anything).Return("", nil).Once()
	store.EXPECT().ClaimOwner(mock.Anything, 42, "Nick").Return(true, nil).Once()
	store.EXPECT().ClearLegacyTokens(mock.Anything).Return(errStoreClosed).Once()

	routes := map[string]plexRoute{"/api/v2/user": userRoute()}

	user, err := testAuth(
		t,
		newPlexServer(t, routes).URL,
		store,
		nil,
	).SignIn(t.Context(), "access-token")

	require.NoError(t, err)
	assert.Equal(t, RoleOwner, user.Role)
}

func TestSignInReportsAFailedStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		expect func(store *mocks.MockStore)
		want   string
	}{
		{
			name: "look up",
			expect: func(store *mocks.MockStore) {
				store.EXPECT().UserRole(mock.Anything, 42).Return("", false, errStoreClosed).Once()
			},
			want: "look up user",
		},
		{
			name: "owner check",
			expect: func(store *mocks.MockStore) {
				store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
				store.EXPECT().HasOwner(mock.Anything).Return(false, errStoreClosed).Once()
			},
			want: "check owner",
		},
		{
			name: "claim",
			expect: func(store *mocks.MockStore) {
				store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
				store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
				store.EXPECT().LegacyToken(mock.Anything).Return("", nil).Once()
				store.EXPECT().
					ClaimOwner(mock.Anything, 42, "Nick").
					Return(false, errStoreClosed).
					Once()
			},
			want: "claim owner",
		},
		{
			name: "claim winner",
			expect: func(store *mocks.MockStore) {
				store.EXPECT().UserRole(mock.Anything, 42).Return("", false, nil).Once()
				store.EXPECT().HasOwner(mock.Anything).Return(false, nil).Once()
				store.EXPECT().LegacyToken(mock.Anything).Return("", nil).Once()
				store.EXPECT().ClaimOwner(mock.Anything, 42, "Nick").Return(false, nil).Once()
				store.EXPECT().UserRole(mock.Anything, 42).Return("", false, errStoreClosed).Once()
			},
			want: "look up user",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := mocks.NewMockStore(t)
			test.expect(store)

			routes := map[string]plexRoute{"/api/v2/user": userRoute()}

			_, err := testAuth(
				t,
				newPlexServer(t, routes).URL,
				store,
				nil,
			).SignIn(t.Context(), "access-token")

			require.ErrorIs(t, err, errStoreClosed)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestSignInBindsTheOnlyDiscoveredServer(t *testing.T) {
	t.Parallel()

	origin, reachable, _ := reachablePlexTV(t)

	store := mocks.NewMockStore(t)
	store.EXPECT().UserRole(mock.Anything, 42).Return("owner", true, nil).Once()
	store.EXPECT().TouchLogin(mock.Anything, 42, "Nick").Return(nil).Once()
	store.EXPECT().SaveSelectedServer(mock.Anything, reachable).Return(nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(plex.EmptyServer(), false).Once()
	bound.EXPECT().Set(reachable).Once()

	_, err := testAuth(t, origin, store, bound).SignIn(t.Context(), "access-token")

	require.NoError(t, err)
}

func TestForgetServerClearsTheSelectionAndItsRecord(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().ClearSelectedServer(mock.Anything).Return(nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Clear().Once()

	auth := New("outtake", "test-client", "http://localhost:8080", store, bound)

	require.NoError(t, auth.ForgetServer(t.Context()))
}

func TestForgetServerReportsAFailedClear(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)
	store.EXPECT().ClearSelectedServer(mock.Anything).Return(errStoreClosed).Once()

	// The binding has no expectations, so clearing it fails the test.
	bound := mocks.NewMockServerBinding(t)

	auth := New("outtake", "test-client", "http://localhost:8080", store, bound)

	err := auth.ForgetServer(t.Context())
	require.ErrorIs(t, err, errStoreClosed)
	require.ErrorContains(t, err, "clear selected server")
}

func TestForgetServerToleratesAbsentCollaborators(t *testing.T) {
	t.Parallel()

	auth := New("outtake", "test-client", "http://localhost:8080", nil, nil)

	require.NoError(t, auth.ForgetServer(t.Context()))
}

func TestBindServerBindsTheReachableConnectionOfTheOnlyServer(t *testing.T) {
	t.Parallel()

	origin, reachable, _ := reachablePlexTV(t)

	store := mocks.NewMockStore(t)
	store.EXPECT().SaveSelectedServer(mock.Anything, reachable).Return(nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(plex.EmptyServer(), false).Once()
	bound.EXPECT().Set(reachable).Once()

	testAuth(t, origin, store, bound).bindServer(t.Context(), "access-token")
}

func TestBindServerSkipsAServerWithNoReachableConnection(t *testing.T) {
	t.Parallel()

	xml := fmt.Sprintf(`<MediaContainer>
	<Device name="Attic" owned="1" clientIdentifier="machine-1" provides="server" accessToken="discovered-token">
		<Connection protocol="http" address="127.0.0.1" port="%d" local="1"/>
	</Device>
</MediaContainer>`, deadPort(t))

	// The binding only answers Get, so binding a dead connection fails the test.
	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(plex.EmptyServer(), false).Once()

	routes := map[string]plexRoute{"/api/resources": {status: http.StatusOK, body: xml}}

	testAuth(
		t,
		newPlexServer(t, routes).URL,
		mocks.NewMockStore(t),
		bound,
	).bindServer(t.Context(), "access-token")
}

func TestBindServerReportsAFailedDiscovery(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)

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

	store := mocks.NewMockStore(t)

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

	store := mocks.NewMockStore(t)

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().
		Get().
		Return(plex.EmptyServer(), false).
		Once()

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
	<Device name="Attic" owned="1" address="10.0.0.5" port="32400" accessToken="discovered-token">
		<Connection protocol="http" address="192.168.1.9" port="32400" local="1"/>
	</Device>
	<Device name="Cellar" owned="1" address="10.0.0.6" port="32400" accessToken="other-token">
		<Connection protocol="http" address="192.168.1.10" port="32400"/>
	</Device>
</MediaContainer>`},
	}

	auth := testAuth(t, newPlexServer(t, routes).URL, store, bound)
	auth.bindServer(t.Context(), "access-token")
}

// TestBindServerLeavesASelectionItCannotIdentify covers a bound server it
// cannot identify.
//
// The server has no stored machine identifier and does not answer /identity.
// Nothing is discovered, set, or saved.
func TestBindServerLeavesASelectionItCannotIdentify(t *testing.T) {
	t.Parallel()

	store := mocks.NewMockStore(t)

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

	store := mocks.NewMockStore(t)

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

	origin, reachable, _ := reachablePlexTV(t)

	store := mocks.NewMockStore(t)
	store.EXPECT().SaveSelectedServer(mock.Anything, reachable).Return(errStoreClosed).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(plex.EmptyServer(), false).Once()
	bound.EXPECT().Set(reachable).Once()

	testAuth(t, origin, store, bound).bindServer(t.Context(), "access-token")
}

func TestChooseServerResolvesADiscoveredConnection(t *testing.T) {
	t.Parallel()

	origin, reachable, _ := reachablePlexTV(t)

	server, err := testAuth(
		t,
		origin,
		nil,
		nil,
	).ChooseServer(t.Context(), "access-token", plex.SelectionKey(reachable))

	require.NoError(t, err)
	assert.Equal(t, reachable, server, "the token comes from discovery")
}

func TestChooseServerRefusesAKeyNoDiscoveredServerHas(t *testing.T) {
	t.Parallel()

	origin, _, _ := reachablePlexTV(t)

	_, err := testAuth(
		t,
		origin,
		nil,
		nil,
	).ChooseServer(t.Context(), "access-token", "machine-1 http://evil.example:80")

	require.ErrorIs(t, err, ErrServerNotFound)
}

func TestChooseServerRefusesAConnectionThatDoesNotAnswer(t *testing.T) {
	t.Parallel()

	origin, _, dead := reachablePlexTV(t)

	_, err := testAuth(
		t,
		origin,
		nil,
		nil,
	).ChooseServer(t.Context(), "access-token", plex.SelectionKey(dead))

	require.ErrorIs(t, err, ErrServerUnreachable)
}

func TestChooseServerReportsAFailedDiscovery(t *testing.T) {
	t.Parallel()

	_, err := testAuth(
		t,
		newPlexServer(t, nil).URL,
		nil,
		nil,
	).ChooseServer(t.Context(), "access-token", "any")

	require.ErrorContains(t, err, "discover servers")
}

func TestChooseCustomURLUsesTheTokenOfTheMatchingServer(t *testing.T) {
	t.Parallel()

	origin, reachable, _ := reachablePlexTV(t)

	server, err := testAuth(
		t,
		origin,
		nil,
		nil,
	).ChooseCustomURL(t.Context(), "access-token", origin)

	require.NoError(t, err)
	assert.Equal(t, "discovered-token", server.Token)
	assert.Equal(t, "machine-1", server.MachineID)
	assert.Equal(t, reachable.Port, server.Port)
	assert.Equal(t, "Attic", server.Name)
}

func TestChooseCustomURLSendsNoTokenToTheHostItChecks(t *testing.T) {
	t.Parallel()

	origin, _, _ := reachablePlexTV(t)

	tokens := make(chan string, 1)

	custom := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// Only the first request is kept. A send that waited for the test
			// to read would hold the handler, and Close waits for handlers.
			select {
			case tokens <- request.Header.Get("X-Plex-Token"):
			default:
			}

			_, _ = writer.Write([]byte(`{"MediaContainer":{"machineIdentifier":"someone-else"}}`))
		}),
	)
	t.Cleanup(custom.Close)

	_, err := testAuth(t, origin, nil, nil).ChooseCustomURL(t.Context(), "access-token", custom.URL)

	require.ErrorIs(
		t,
		err,
		ErrServerNotFound,
		"a host that is not one of the account's servers is refused",
	)
	assert.Empty(t, <-tokens, "no token reaches a host before it is verified")
}

func TestChooseCustomURLRefusesAURLThatIsNotHTTP(t *testing.T) {
	t.Parallel()

	for _, rawURL := range []string{"ftp://plex.example.com", "plex.example.com", "", "http://"} {
		_, err := testAuth(t, "", nil, nil).ChooseCustomURL(t.Context(), "access-token", rawURL)

		require.ErrorIs(t, err, ErrInvalidServerURL, rawURL)
	}
}

func TestChooseCustomURLRefusesAHostThatDoesNotAnswer(t *testing.T) {
	t.Parallel()

	origin, _, _ := reachablePlexTV(t)

	_, err := testAuth(t, origin, nil, nil).ChooseCustomURL(
		t.Context(), "access-token", fmt.Sprintf("http://127.0.0.1:%d", deadPort(t)),
	)

	require.ErrorIs(t, err, ErrServerUnreachable)
}

// TestBindServerRefreshesTheTokenOfTheBoundServer covers a sign-in while the
// bound token is refused.
//
// A server is bound with a token Plex refuses. Discovery's token for the same
// machine replaces it, and the bound connection, here a custom URL, is kept.
func TestBindServerRefreshesTheTokenOfTheBoundServer(t *testing.T) {
	t.Parallel()

	origin, _, _ := reachablePlexTV(t)

	stale := plex.Server{
		Name: "Attic", Address: "plex.example.com", Port: 443, Token: "stale-token",
		Scheme: "https", Local: false, MachineID: "machine-1", Relay: false,
	}

	refreshed := stale

	refreshed.Token = "discovered-token"

	store := mocks.NewMockStore(t)
	store.EXPECT().SaveSelectedServer(mock.Anything, refreshed).Return(nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(stale, true).Once()
	bound.EXPECT().Set(refreshed).Once()

	testAuth(t, origin, store, bound).bindServer(t.Context(), "access-token")
}

// TestBindServerIdentifiesALegacyBindingBeforeRefreshing covers a binding
// without a machine identifier.
//
// The binding was stored before its machine identifier was recorded. The bound
// connection is asked for its identity, and the refreshed binding records it.
func TestBindServerIdentifiesALegacyBindingBeforeRefreshing(t *testing.T) {
	t.Parallel()

	origin, reachable, _ := reachablePlexTV(t)

	legacy := reachable

	legacy.Token = "stale-token"
	legacy.MachineID = ""

	store := mocks.NewMockStore(t)
	store.EXPECT().SaveSelectedServer(mock.Anything, reachable).Return(nil).Once()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(legacy, true).Once()
	bound.EXPECT().Set(reachable).Once()

	testAuth(t, origin, store, bound).bindServer(t.Context(), "access-token")
}

// TestBindServerKeepsACurrentToken covers a bound server whose token discovery
// still reports.
//
// Nothing is set or saved.
func TestBindServerKeepsACurrentToken(t *testing.T) {
	t.Parallel()

	origin, reachable, _ := reachablePlexTV(t)

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(reachable, true).Once()

	testAuth(t, origin, mocks.NewMockStore(t), bound).bindServer(t.Context(), "access-token")
}

// TestBindServerLeavesAServerTheAccountDoesNotList covers a bound server the
// account does not list.
//
// The bound server is not among the signed-in account's servers. No other
// server's token is put on it.
func TestBindServerLeavesAServerTheAccountDoesNotList(t *testing.T) {
	t.Parallel()

	origin, reachable, _ := reachablePlexTV(t)

	other := reachable

	other.MachineID = "machine-2"
	other.Token = "stale-token"

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(other, true).Once()

	testAuth(t, origin, mocks.NewMockStore(t), bound).bindServer(t.Context(), "access-token")
}

// TestBindServerDoesNotBindAServerSharedWithTheAccount covers a first sign-in
// by an account that owns no server.
//
// Its only server is shared with it, so nothing is bound.
func TestBindServerDoesNotBindAServerSharedWithTheAccount(t *testing.T) {
	t.Parallel()

	shared := `<MediaContainer>
	<Device name="Friend" owned="0" clientIdentifier="machine-2" provides="server" accessToken="shared-token">
		<Connection protocol="http" address="192.168.1.10" port="32400" local="1"/>
	</Device>
</MediaContainer>`

	// The binding only answers Get, so binding the shared server fails the test.
	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(plex.EmptyServer(), false).Once()

	routes := map[string]plexRoute{"/api/resources": {status: http.StatusOK, body: shared}}

	testAuth(
		t,
		newPlexServer(t, routes).URL,
		mocks.NewMockStore(t),
		bound,
	).bindServer(t.Context(), "access-token")
}

// TestBindServerKeepsTheOwnersTokenWhenASharedAccountSignsIn covers a sign-in
// by an account the bound server is shared with.
//
// Its token for the server reaches only what is shared with it, so the
// owner's token stays.
func TestBindServerKeepsTheOwnersTokenWhenASharedAccountSignsIn(t *testing.T) {
	t.Parallel()

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().Get().Return(testFriendServer, true).Once()

	routes := map[string]plexRoute{
		"/api/resources": {status: http.StatusOK, body: testSharedServerXML},
	}

	testAuth(
		t,
		newPlexServer(t, routes).URL,
		mocks.NewMockStore(t),
		bound,
	).bindServer(t.Context(), "access-token")
}

// TestTokenRejectedFollowsTheBinding covers the refused-token check pages show.
//
// It is false with no binding, and otherwise asks the binding.
func TestTokenRejectedFollowsTheBinding(t *testing.T) {
	t.Parallel()

	assert.False(t, testAuth(t, "", nil, nil).TokenRejected())

	bound := mocks.NewMockServerBinding(t)
	bound.EXPECT().TokenRejected().Return(true).Once()

	assert.True(t, testAuth(t, "", nil, bound).TokenRejected())
}
