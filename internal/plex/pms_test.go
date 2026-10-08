// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPMSServer starts a loopback server whose response is chosen by path.
//
// Parameters:
//   - t: The test context.
//   - responses: Map of request path to the HTTP status the path answers with.
//
// Returns:
//   - ts: A server that answers every other path with the JSON in fallback.
func testPMSServer(t *testing.T, responses map[string]int) *httptest.Server {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, known := responses[r.URL.Path]
		if !known {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
	}))
	t.Cleanup(ts.Close)

	return ts
}

// testPMSClientFor builds a client and a PMS bound to the loopback server.
//
// Parameters:
//   - t: The test context.
//   - ts: The loopback server standing in for a PMS.
//
// Returns:
//   - client: A client carrying "srv-token" as its own token.
//   - server: A Server pointing at ts with an empty token.
func testPMSClientFor(t *testing.T, ts *httptest.Server) (*Client, Server) {
	t.Helper()

	host, port := extractAddrPort(t, ts)
	client := NewClient(ClientConfig{
		Product:  productName,
		ClientID: "test",
		Token:    "srv-token",
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})

	return client, Server{
		Name:    "Test",
		Address: host,
		Port:    port,
		Token:   "",
		Scheme:  httpScheme,
		Local:   false,
	}
}

func TestServerBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		server Server
		want   string
	}{
		{
			name:   "blank scheme defaults to https",
			server: Server{Address: "plex.local", Port: 32400},
			want:   "https://plex.local:32400",
		},
		{
			name:   "https default port is omitted",
			server: Server{Address: "plex.local", Port: httpsPort, Scheme: defaultScheme},
			want:   "https://plex.local",
		},
		{
			name:   "http default port is omitted",
			server: Server{Address: "plex.local", Port: httpPort, Scheme: httpScheme},
			want:   "http://plex.local",
		},
		{
			name:   "http with a plex port keeps the port",
			server: Server{Address: "192.168.1.5", Port: defaultPlexPort, Scheme: httpScheme},
			want:   "http://192.168.1.5:32400",
		},
		{
			name:   "ipv6 address is bracketed",
			server: Server{Address: "::1", Port: defaultPlexPort, Scheme: httpScheme},
			want:   "http://[::1]:32400",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, serverBaseURL(test.server))
		})
	}
}

func TestGetPMSTokenPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		serverToken  string
		wantToken    string
		wantAccepted string
	}{
		{
			name:         "server token is preferred",
			serverToken:  "srv-token",
			wantToken:    "srv-token",
			wantAccepted: acceptJSON,
		},
		{
			name:         "blank server token falls back to the client token",
			serverToken:  "",
			wantToken:    "client-token",
			wantAccepted: acceptJSON,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var seen string

			ts := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					seen = r.Header.Get(headerPlexToken)

					w.WriteHeader(http.StatusOK)
				},
			))
			defer ts.Close()

			client, server := testPMSClientFor(t, ts)

			client.Token = "client-token"
			server.Token = test.serverToken

			_, err := client.getPMS(t.Context(), server, "/library/sections/all", "")
			require.NoError(t, err)
			assert.Equal(t, test.wantToken, seen)
		})
	}
}

func TestGetPMSFailureStatus(t *testing.T) {
	t.Parallel()

	ts := testPMSServer(t, map[string]int{"/library/metadata/100": http.StatusNotFound})

	client, server := testPMSClientFor(t, ts)

	_, err := client.getPMS(t.Context(), server, "/library/metadata/100", "")
	require.ErrorIs(t, err, ErrServerReturnedError)
	require.ErrorContains(t, err, "404")
}

func TestGetMediaItem(t *testing.T) {
	t.Parallel()

	// testItemBody is a populated single-item MediaContainer.
	const testItemBody = `{"MediaContainer":{"Metadata":[
		{"ratingKey":"100","key":"/library/metadata/100","title":"Test Movie",
		 "type":"movie","duration":7200000,"thumb":"/library/metadata/100/thumb/1",
		 "year":1999,"librarySectionID":"1",
		 "Media":[{"Part":[{"file":""},{"file":"/data/movies/Test Movie.mkv"}]}]}
	]}}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/metadata/100", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(testItemBody))
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	item, err := client.GetMediaItem(t.Context(), server, "100")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "100", item.ID)
	assert.Equal(t, "Test Movie", item.Title)
	assert.Equal(t, "movie", item.Type)
	assert.InEpsilon(t, 7200.0, item.Duration, 0.01)
	assert.Equal(t, 1999, item.Year)
	assert.Equal(t, "1", item.LibraryID)
	assert.Equal(t, "Test Movie (1999)", item.DisplayTitle())
	assert.Equal(t, "/data/movies/Test Movie.mkv", item.FilePath,
		"the first part with a file is the one a clip is cut from")
}

func TestGetMediaItemKeysByRequestedID(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
			{"key":"/library/metadata/555/children","title":"Named Only","type":"show"}
		]}}`))
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	item, err := client.GetMediaItem(t.Context(), server, "555")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "555", item.ID)
	assert.Equal(t, "Named Only", item.Title)
	assert.Empty(t, item.FilePath, "an item with no media part names no file")
}

func TestGetMediaItem_EmptyContainer(t *testing.T) {
	t.Parallel()

	// testEmptyContainerBody is a MediaContainer carrying no Metadata rows.
	const testEmptyContainerBody = `{"MediaContainer":{}}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(testEmptyContainerBody))
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	item, err := client.GetMediaItem(t.Context(), server, "100")
	require.ErrorIs(t, err, ErrNoFilePathFound)
	assert.Nil(t, item)
}

func TestGetMediaItem_RowsWithoutIdentityAreNotEmpty(t *testing.T) {
	t.Parallel()

	// testNoMetadataContainerBody is a MediaContainer whose rows carry no identity.
	const testNoMetadataContainerBody = `{"MediaContainer":{"Metadata":[
		{"key":"/library/sections/1/title","title":"Movies","type":"directory"}
	]}}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(testNoMetadataContainerBody))
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	item, err := client.GetMediaItem(t.Context(), server, "100")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "100", item.ID)
	assert.Equal(t, "Movies", item.Title)
	assert.Equal(t, "unknown", item.Type)
}

func TestGetMediaItem_ServerError(t *testing.T) {
	t.Parallel()

	ts := testPMSServer(t, map[string]int{"/library/metadata/100": http.StatusUnauthorized})

	client, server := testPMSClientFor(t, ts)

	_, err := client.GetMediaItem(t.Context(), server, "100")
	require.ErrorIs(t, err, ErrServerReturnedError)
	require.ErrorContains(t, err, "get media item")
}

func TestGetMediaItem_DecodeError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	_, err := client.GetMediaItem(t.Context(), server, "100")
	require.ErrorIs(t, err, errEmptyBody)
	require.ErrorContains(t, err, "decode media item")
}

func TestGetChildrenPage(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/metadata/10/children", r.URL.Path)
		assert.Equal(t, "48", r.URL.Query().Get("X-Plex-Container-Start"))
		assert.Equal(t, "48", r.URL.Query().Get("X-Plex-Container-Size"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"size":2,"totalSize":120,"offset":48,"Metadata":[
			{"ratingKey":"11","title":"Season 1","type":"season"},
			{"ratingKey":"12","title":"Season 2","type":"season"}
		]}}`))
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	page, err := client.GetChildrenPage(t.Context(), server, "10", 48, 48)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Equal(t, "Season 1", page.Items[0].Title)
	assert.Equal(t, 120, page.Total)
	assert.Equal(t, 48, page.Start)
	assert.Equal(t, 48, page.Size)
}

func TestGetChildrenPage_TotalFallsBackToSizeThenWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		start     int
		wantTotal int
	}{
		{
			name:      "totalSize is preferred",
			body:      `{"MediaContainer":{"size":1,"totalSize":99,"Metadata":[{"ratingKey":"11"}]}}`,
			wantTotal: 99,
		},
		{
			name:      "size is used when totalSize is absent",
			body:      `{"MediaContainer":{"size":7,"Metadata":[{"ratingKey":"11"}]}}`,
			wantTotal: 7,
		},
		{
			name:      "window is used when the server reports no size",
			body:      `{"MediaContainer":{"Metadata":[{"ratingKey":"11"},{"ratingKey":"12"}]}}`,
			start:     5,
			wantTotal: 7,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)

				_, _ = w.Write([]byte(test.body))
			}))
			defer ts.Close()

			client, server := testPMSClientFor(t, ts)

			page, err := client.GetChildrenPage(t.Context(), server, "10", test.start, 48)
			require.NoError(t, err)
			assert.Equal(t, test.wantTotal, page.Total)
		})
	}
}

func TestGetChildrenPage_AllLeavesFallback(t *testing.T) {
	t.Parallel()

	var requested []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)

		if strings.HasSuffix(r.URL.Path, "/children") {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		assert.Equal(t, "24", r.URL.Query().Get("X-Plex-Container-Start"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"size":1,"Metadata":[
			{"ratingKey":"21","title":"Fallout","type":"episode"}
		]}}`))
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	page, err := client.GetChildrenPage(t.Context(), server, "10", 24, 48)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Fallout", page.Items[0].Title)
	assert.Equal(t, []string{
		"/library/metadata/10/children",
		"/library/metadata/10/allLeaves",
	}, requested)
}

func TestGetChildrenPage_BothRequestsFail(t *testing.T) {
	t.Parallel()

	ts := testPMSServer(t, map[string]int{
		"/library/metadata/10/children":  http.StatusInternalServerError,
		"/library/metadata/10/allLeaves": http.StatusInternalServerError,
	})

	client, server := testPMSClientFor(t, ts)

	page, err := client.GetChildrenPage(t.Context(), server, "10", 0, 48)
	require.ErrorIs(t, err, ErrServerReturnedError)
	require.ErrorContains(t, err, "get children")
	assert.Empty(t, page.Items)
}

func TestGetChildrenPage_DecodeError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	_, err := client.GetChildrenPage(t.Context(), server, "10", 0, 48)
	require.ErrorIs(t, err, errEmptyBody)
	require.ErrorContains(t, err, "decode children")
}

func TestGetChildren_AllLeavesFallback(t *testing.T) {
	t.Parallel()

	var requested string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path

		if strings.HasSuffix(r.URL.Path, "/children") {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
			{"key":"/library/metadata/31/children","title":"Pilot","type":"episode"}
		]}}`))
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	items, err := client.GetChildren(t.Context(), server, "10")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Pilot", items[0].Title)
	assert.Equal(t, "/library/metadata/10/allLeaves", requested)
}

func TestGetChildren_BothRequestsFail(t *testing.T) {
	t.Parallel()

	ts := testPMSServer(t, map[string]int{
		"/library/metadata/10/children":  http.StatusInternalServerError,
		"/library/metadata/10/allLeaves": http.StatusInternalServerError,
	})

	client, server := testPMSClientFor(t, ts)

	_, err := client.GetChildren(t.Context(), server, "10")
	require.ErrorIs(t, err, ErrServerReturnedError)
	require.ErrorContains(t, err, "get children")
}

func TestGetChildren_DecodeError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	_, err := client.GetChildren(t.Context(), server, "10")
	require.ErrorIs(t, err, errEmptyBody)
	require.ErrorContains(t, err, "decode children")
}

func TestGetChildren_SkipsRowsWithoutIdentity(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
			{"key":"/library/sections/1/title","title":"Movies","type":"directory"},
			{"ratingKey":"41","title":"Season 1","type":"season"}
		]}}`))
	}))
	defer ts.Close()

	client, server := testPMSClientFor(t, ts)

	items, err := client.GetChildren(t.Context(), server, "10")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Season 1", items[0].Title)
}
