// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// TestSrvToken is the test server token.
	testSrvToken = "srv-token"
	// TestServerClient is the test server client ID.
	testServerClient = "test"
	// TestServerName is the test server name.
	testServerName = "Test"
)

func extractAddrPort(t *testing.T, server *httptest.Server) (string, int) {
	t.Helper()

	addr := server.Listener.Addr().String()
	parts := strings.Split(addr, ":")
	require.Len(t, parts, 2, "expected host:port format")

	port, err := strconv.Atoi(parts[1])
	require.NoError(t, err)

	return parts[0], port
}

func TestGetLibraries(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/sections/all", r.URL.Path)

		if got := r.Header.Get("Accept"); got != acceptJSON {
			t.Fatalf("Accept=%q want %q", got, acceptJSON)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
			{"key":"1","title":"Movies","type":"movie"},
			{"key":"2","title":"TV Shows","type":"show"}
		]}}`))
	}))

	defer ts.Close()

	host, port := extractAddrPort(t, ts)
	c := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: testServerClient,
			Token:    testSrvToken,
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    testServerName,
		Address: host,
		Port:    port,
		Token:   testSrvToken,
		Scheme:  httpScheme,
		Local:   false,
	}

	libs, err := c.GetLibraries(t.Context(), server)
	require.NoError(t, err)
	assert.Len(t, libs, 2)
	assert.Equal(t, "Movies", libs[0].Title)
	assert.Equal(t, "TV Shows", libs[1].Title)
}

func TestGetMedia(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
			{"ratingKey":"100","key":"/library/metadata/100","title":"Test Movie","duration":7200000,"thumb":"/library/metadata/100/thumb/1","type":"movie"},
			{"ratingKey":200,"key":"/library/metadata/200/children","title":"A Show","type":"show","thumb":"/library/metadata/200/thumb/1"}
		]}}`))
	}))

	defer ts.Close()

	host, port := extractAddrPort(t, ts)
	c := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: testServerClient,
			Token:    testSrvToken,
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    testServerName,
		Address: host,
		Port:    port,
		Token:   testSrvToken,
		Scheme:  httpScheme,
		Local:   false,
	}

	items, err := c.GetMedia(t.Context(), server, "1")
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.InEpsilon(t, 7200.0, items[0].Duration, 0.01)
	assert.Equal(t, "Test Movie", items[0].Title)
	assert.Equal(t, "200", items[1].ID)
	assert.Equal(t, "show", items[1].Type)
}

func TestGetMediaPath(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
			{"Media":[{"Part":[{"file":"/media/movies/Test Movie.mkv"}]}]}
		]}}`))
	}))

	defer ts.Close()

	host, port := extractAddrPort(t, ts)
	c := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: testServerClient,
			Token:    testSrvToken,
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    testServerName,
		Address: host,
		Port:    port,
		Token:   testSrvToken,
		Scheme:  httpScheme,
		Local:   false,
	}

	path, err := c.GetMediaPath(t.Context(), server, "100")
	require.NoError(t, err)
	assert.Equal(t, "/media/movies/Test Movie.mkv", path)
}

func TestGetSessions(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer size="1">
				<Video title="Now Playing" duration="3600000">
					<Session id="sess-123"/>
				</Video>
			</MediaContainer>`))
	}))

	defer ts.Close()

	c := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: testServerClient,
			Token:    testSrvToken,
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)

	c.baseURL.Scheme = httpScheme
	c.baseURL.Host = ts.Listener.Addr().String()

	sessions, err := c.GetSessions(t.Context())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.InEpsilon(t, 3600.0, sessions[0].Duration, 0.01)
	assert.Equal(t, "Now Playing", sessions[0].Title)
}

func TestDiscoverServers(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
			<MediaContainer>
				<Device name="My Server" address="192.168.1.100" port="32400" accessToken="discovered-token">
					<Connection address="192.168.1.100" port="32400"/>
				</Device>
			</MediaContainer>`))
	}))

	defer ts.Close()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: testServerClient,
		Token:    "",
		Timeout:  0,
		BaseURL:  "",
	})

	c.baseURL.Scheme = httpScheme
	c.baseURL.Host = ts.Listener.Addr().String()

	servers, err := c.DiscoverServers(t.Context())
	require.NoError(t, err)
	require.Len(t, servers, 1)
	assert.Equal(t, "My Server", servers[0].Name)
	assert.Equal(t, "192.168.1.100", servers[0].Address)
	assert.Equal(t, 32400, servers[0].Port)
	assert.Equal(t, "discovered-token", servers[0].Token)
}

func TestPing(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	defer ts.Close()

	host, port := extractAddrPort(t, ts)
	c := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: testServerClient,
			Token:    testSrvToken,
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    testServerName,
		Address: host,
		Port:    port,
		Token:   testSrvToken,
		Scheme:  httpScheme,
		Local:   false,
	}

	err := c.Ping(t.Context(), server)
	require.NoError(t, err)
}

func TestPing_Unreachable(t *testing.T) {
	t.Parallel()

	c := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: testServerClient,
			Token:    testSrvToken,
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    testServerName,
		Address: "127.0.0.1",
		Port:    1,
		Token:   testSrvToken,
		Scheme:  httpScheme,
		Local:   false,
	}

	err := c.Ping(t.Context(), server)
	assert.Error(t, err)
}

func TestGetSessionsOnServer(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
			{"ratingKey":"42","title":"Now Playing","type":"movie","duration":3600000,"viewOffset":120000,"Session":{"id":"sess-123"}}
		]}}`))
	}))
	defer ts.Close()

	host, port := extractAddrPort(t, ts)
	c := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: testServerClient,
			Token:    testSrvToken,
			Timeout:  0,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    testServerName,
		Address: host,
		Port:    port,
		Token:   testSrvToken,
		Scheme:  httpScheme,
		Local:   false,
	}

	sessions, err := c.GetSessionsOnServer(t.Context(), server)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "42", sessions[0].MediaItem.ID)
	assert.InEpsilon(t, 120.0, sessions[0].ViewOffset, 0.01)
}

func TestSearchOnServer(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/hubs/search", r.URL.Path)
		assert.Equal(t, "alpha", r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[
			{"title":"Movies","type":"movie","Metadata":[
				{"ratingKey":"9","title":"Alpha Movie","type":"movie","duration":1000}
			]}
		]}}`))
	}))
	defer ts.Close()

	host, port := extractAddrPort(t, ts)
	c := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: testServerClient,
			Token:    testSrvToken,
			Timeout:  0,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    testServerName,
		Address: host,
		Port:    port,
		Token:   testSrvToken,
		Scheme:  httpScheme,
		Local:   false,
	}

	items, err := c.SearchOnServer(t.Context(), server, "alpha")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Alpha Movie", items[0].Title)
}
