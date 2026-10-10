// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex/decode/pms"
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

func testPMSClient(t *testing.T, ts *httptest.Server) (*Client, Server) {
	t.Helper()

	host, port := extractAddrPort(t, ts)
	client := NewClient(ClientConfig{
		Product:  productName,
		ClientID: "test",
		Token:    "srv-token",
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})
	server := Server{
		Name:    "Test",
		Address: host,
		Port:    port,
		Token:   "srv-token",
		Scheme:  httpScheme,
		Local:   false,
	}

	return client, server
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
			ClientID: "test",
			Token:    "srv-token",
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    "Test",
		Address: host,
		Port:    port,
		Token:   "srv-token",
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
			ClientID: "test",
			Token:    "srv-token",
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    "Test",
		Address: host,
		Port:    port,
		Token:   "srv-token",
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

func TestGetMediaPageSendsContainerQuery(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "48", r.URL.Query().Get("X-Plex-Container-Start"))
		assert.Equal(t, "48", r.URL.Query().Get("X-Plex-Container-Size"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"size":1,"totalSize":200,"offset":48,"Metadata":[
			{"ratingKey":"100","title":"Paged","type":"movie","year":1999}
		]}}`))
	}))
	defer ts.Close()

	c, server := testPMSClient(t, ts)

	page, err := c.GetMediaPage(t.Context(), server, "1", 48, 48, "")
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, 200, page.Total)
	assert.Equal(t, 1999, page.Items[0].Year)
	assert.Equal(t, "Paged (1999)", page.Items[0].DisplayTitle())
}

func TestGetMediaPageSendsSort(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "0", r.URL.Query().Get("X-Plex-Container-Start"))
		assert.Equal(t, "48", r.URL.Query().Get("X-Plex-Container-Size"))
		assert.Equal(t, "titleSort:asc", r.URL.Query().Get("sort"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"size":0,"totalSize":0,"Metadata":[]}}`))
	}))
	defer ts.Close()

	c, server := testPMSClient(t, ts)

	_, err := c.GetMediaPage(t.Context(), server, "1", 0, 48, "titleSort:asc")
	require.NoError(t, err)
}

func TestMediaListQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		start int
		size  int
		sort  string
		want  string
	}{
		{name: "empty", want: ""},
		{
			name:  "page only",
			start: 48,
			size:  48,
			want:  "X-Plex-Container-Size=48&X-Plex-Container-Start=48",
		},
		{
			name: "sort only",
			sort: "year:desc",
			want: "sort=year%3Adesc",
		},
		{
			name:  "page and sort",
			start: 0,
			size:  48,
			sort:  "titleSort:asc",
			want:  "X-Plex-Container-Size=48&X-Plex-Container-Start=0&sort=titleSort%3Aasc",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, mediaListQuery(test.start, test.size, test.sort))
		})
	}
}

func TestGetFirstCharacters(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/sections/1/firstCharacter", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
			{"key":"/library/sections/1/firstCharacter/%23","title":"#","size":3},
			{"key":"/library/sections/1/firstCharacter/A","title":"A","size":40}
		]}}`))
	}))
	defer ts.Close()

	c, server := testPMSClient(t, ts)

	index, err := c.GetFirstCharacters(t.Context(), server, "1")
	require.NoError(t, err)
	require.Equal(t, []LetterIndex{
		{Title: "#", Size: 3},
		{Title: "A", Size: 40},
	}, index)
}

func TestGetYears(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/sections/1/year", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
			{"title":"2024","size":12},
			{"title":"1995","size":4}
		]}}`))
	}))
	defer ts.Close()

	c, server := testPMSClient(t, ts)

	index, err := c.GetYears(t.Context(), server, "1")
	require.NoError(t, err)
	require.Equal(t, []LetterIndex{
		{Title: "2024", Size: 12},
		{Title: "1995", Size: 4},
	}, index)
}

func TestGetSectionIndexRejectsUnknown(t *testing.T) {
	t.Parallel()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: "test",
		Token:    "srv-token",
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})

	_, err := c.GetSectionIndex(t.Context(), Server{}, "1", "genre")
	require.ErrorIs(t, err, ErrUnsupportedSectionIndex)
}

func TestMediaPageNormalizesNegativeStart(t *testing.T) {
	t.Parallel()

	page := mediaPage(pms.Container{}, -5, 48)
	assert.Equal(t, 0, page.Start)
	assert.Equal(t, 0, page.Total)
	assert.Equal(t, 48, page.Size)
}

func TestGetMediaPathRefusesAnIDThatIsNotARatingKey(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		assert.Fail(t, "no request may reach the server", r.URL.String())
	}))
	t.Cleanup(ts.Close)

	host, port := extractAddrPort(t, ts)
	client := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: "test",
			Token:    "srv-token",
			Timeout:  5 * time.Second,
		},
	)
	server := Server{
		Name:    "Test",
		Address: host,
		Port:    port,
		Token:   "srv-token",
		Scheme:  httpScheme,
	}

	for _, id := range []string{"", "invalid-id", "1/../../sections/1/refresh", "1?includeChildren=1", "-1", "1 2"} {
		_, err := client.GetMediaPath(t.Context(), server, id)
		require.ErrorIs(t, err, ErrInvalidMediaID, id)
	}
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
			ClientID: "test",
			Token:    "srv-token",
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    "Test",
		Address: host,
		Port:    port,
		Token:   "srv-token",
		Scheme:  httpScheme,
		Local:   false,
	}

	path, err := c.GetMediaPath(t.Context(), server, "100")
	require.NoError(t, err)
	assert.Equal(t, "/media/movies/Test Movie.mkv", path)
}

func TestDiscoverServers(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, acceptXML, r.Header.Get("Accept"),
			"plex.tv returns JSON for Accept: application/json, and this decoder reads XML")

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
		ClientID: "test",
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
			ClientID: "test",
			Token:    "srv-token",
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    "Test",
		Address: host,
		Port:    port,
		Token:   "srv-token",
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
			ClientID: "test",
			Token:    "srv-token",
			Timeout:  5 * time.Second,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    "Test",
		Address: "127.0.0.1",
		Port:    1,
		Token:   "srv-token",
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
			{"ratingKey":"42","title":"Now Playing","type":"movie","duration":3600000,"viewOffset":120000,"Session":{"id":"sess-123"},"Player":{"state":"paused"}}
		]}}`))
	}))
	defer ts.Close()

	host, port := extractAddrPort(t, ts)
	c := NewClient(
		ClientConfig{
			Product:  productName,
			ClientID: "test",
			Token:    "srv-token",
			Timeout:  0,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    "Test",
		Address: host,
		Port:    port,
		Token:   "srv-token",
		Scheme:  httpScheme,
		Local:   false,
	}

	sessions, err := c.GetSessionsOnServer(t.Context(), server)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "42", sessions[0].MediaItem.ID)
	assert.InEpsilon(t, 120.0, sessions[0].ViewOffset, 0.01)
	assert.Equal(t, StatePaused, sessions[0].State, "a paused client's position is exact")
}

func TestSearchOnServer(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/hubs/search", r.URL.Path)
		assert.Equal(t, "alpha", r.URL.Query().Get("query"))
		assert.Equal(t, "5", r.URL.Query().Get("sectionId"))
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
			ClientID: "test",
			Token:    "srv-token",
			Timeout:  0,
			BaseURL:  "",
		},
	)
	server := Server{
		Name:    "Test",
		Address: host,
		Port:    port,
		Token:   "srv-token",
		Scheme:  httpScheme,
		Local:   false,
	}

	items, err := c.SearchOnServer(t.Context(), server, "alpha", "5")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Alpha Movie", items[0].Title)
}

// testPlexTVClient builds a client whose plex.tv origin is the loopback server.
//
// Parameters:
//   - t: The test context.
//   - ts: The loopback server standing in for plex.tv.
//
// Returns:
//   - client: A client bound to ts instead of plex.tv.
func testPlexTVClient(t *testing.T, ts *httptest.Server) *Client {
	t.Helper()

	client := NewClient(ClientConfig{
		Product:  productName,
		ClientID: "test",
		Token:    "srv-token",
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})

	client.baseURL.Scheme = httpScheme
	client.baseURL.Host = ts.Listener.Addr().String()

	return client
}

func TestSearchMedia(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/search", r.URL.Path)
		assert.Equal(t, "alpha beta", r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`<MediaContainer size="2">
			<Video ratingKey="900" key="/library/metadata/900" title="Alpha Movie"
				duration="60000" thumb="/library/metadata/900/thumb/1" type="movie"/>
			<Directory key="/library/sections/1" title="Movies" type="show"/>
		</MediaContainer>`))
	}))
	defer ts.Close()

	items, err := testPlexTVClient(t, ts).SearchMedia(t.Context(), "alpha beta")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "900", items[0].ID)
	assert.Equal(t, "Alpha Movie", items[0].Title)
	assert.Equal(t, "movie", items[0].Type)
	assert.InEpsilon(t, 60.0, items[0].Duration, 0.01)
	assert.Equal(t, "/library/metadata/900/thumb/1", items[0].ThumbPath)
	assert.Empty(t, items[0].LibraryTitle)
}

func TestSearchMedia_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantErr    error
		wantErrHas string
	}{
		{name: "empty body", wantErr: errEmptyBody},
		{
			name:       "malformed xml",
			body:       `<MediaContainer><Video></MediaContainer>`,
			wantErrHas: "decode search",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)

				if test.body != "" {
					_, _ = w.Write([]byte(test.body))
				}
			}))
			defer ts.Close()

			items, err := testPlexTVClient(t, ts).SearchMedia(t.Context(), "alpha")
			require.Error(t, err)

			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			}

			if test.wantErrHas != "" {
				require.ErrorContains(t, err, test.wantErrHas)
			}

			assert.Empty(t, items)
		})
	}
}

// TestPlexTVCallsReportAFailureStatus covers the XML plex.tv calls against
// plex.tv's own error page: each reports the status as ErrPlexError, and a 401
// as ErrUnauthorized, rather than decoding the page as data.
func TestPlexTVCallsReportAFailureStatus(t *testing.T) {
	t.Parallel()

	calls := []struct {
		name string
		call func(ctx context.Context, client *Client) error
	}{
		{
			name: "discover servers",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.DiscoverServers(ctx)
				if err != nil {
					return fmt.Errorf("discover servers: %w", err)
				}

				return nil
			},
		},
		{
			name: "search media",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.SearchMedia(ctx, "alpha")
				if err != nil {
					return fmt.Errorf("search media: %w", err)
				}

				return nil
			},
		},
	}

	statuses := []struct {
		name         string
		status       int
		unauthorized bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, unauthorized: true},
		{name: "not found", status: http.StatusNotFound, unauthorized: false},
	}

	for _, call := range calls {
		for _, status := range statuses {
			t.Run(call.name+" "+status.name, func(t *testing.T) {
				t.Parallel()

				ts := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "application/xml")
						w.WriteHeader(status.status)

						_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
						<hash><error>Not Found</error></hash>`))
					}),
				)
				defer ts.Close()

				err := call.call(t.Context(), testPlexTVClient(t, ts))
				require.ErrorIs(t, err, ErrPlexError)
				assert.Equal(t, status.unauthorized, errors.Is(err, ErrUnauthorized))
				assert.NotContains(t, err.Error(), "decode")
			})
		}
	}
}

// TestLibraryCallsReportARefusedToken covers a server that refuses the token:
// the library list and the media path report ErrUnauthorized rather than
// decoding the refusal page as data.
func TestLibraryCallsReportARefusedToken(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)

		_, _ = w.Write([]byte("<html><body>401 Unauthorized</body></html>"))
	}))
	t.Cleanup(ts.Close)

	client, server := testPMSClientFor(t, ts)

	_, err := client.GetLibraries(t.Context(), server)
	require.ErrorIs(t, err, ErrUnauthorized)
	require.ErrorIs(t, err, ErrServerReturnedError)

	_, err = client.GetMediaPath(t.Context(), server, "42")
	require.ErrorIs(t, err, ErrUnauthorized)
}

// TestLibraryCallsReportAFailureStatus covers any other failure status: it is
// reported as a server error, not as a refused token.
func TestLibraryCallsReportAFailureStatus(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	client, server := testPMSClientFor(t, ts)

	_, err := client.GetLibraries(t.Context(), server)
	require.ErrorIs(t, err, ErrServerReturnedError)
	assert.NotErrorIs(t, err, ErrUnauthorized)
}
