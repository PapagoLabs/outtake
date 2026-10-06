// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidThumbPath(t *testing.T) {
	t.Parallel()

	for path, want := range map[string]bool{
		"/library/metadata/1/thumb/2":                 true,
		"/library/metadata/100/thumb":                 true,
		"/library/metadata/1/art/1700000000":          true,
		"/library/metadata/1/banner/3":                true,
		"/library/metadata/1/clearLogo/4":             true,
		"/library/sections/5/composite/1":             true,
		"/library/collections/9/composite/1700000000": true,
		"/thumb/100":                           false,
		"../etc/passwd":                        false,
		"/library/../etc/passwd":               false,
		"":                                     false,
		"/library/parts/1/1700000000/file.mkv": false,
		"/library/sections/1/refresh":          false,
		"/library/sections/1/all":              false,
		"/photo/:/transcode?url=http://example.com/x": false,
		"/library/metadata/1/thumb/2?url=x":           false,
		"/library/metadata/1%2F..%2Fparts/thumb/2":    false,
		"/library/metadata/1/theme/2":                 false,
		"/library/metadata/one/thumb/2":               false,
		"/library/metadata/1/thumb/2/extra":           false,
	} {
		assert.Equal(t, want, ValidThumbPath(path), path)
	}
}

func TestGetThumbRefusesAResponseThatIsNotAnImage(t *testing.T) {
	t.Parallel()

	for name, contentType := range map[string]string{
		"declared html":   "text/html",
		"undeclared html": "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if contentType == "" {
					w.Header()["Content-Type"] = nil
				} else {
					w.Header().Set("Content-Type", contentType)
				}

				_, _ = w.Write([]byte("<html><body>not an image</body></html>"))
			}))
			t.Cleanup(ts.Close)

			client, server := thumbClient(t, ts)

			_, _, err := client.GetThumb(t.Context(), server, "/library/metadata/1/thumb/2")
			require.ErrorIs(t, err, ErrNotImage)
		})
	}
}

func TestGetThumbDetectsAnUndeclaredImage(t *testing.T) {
	t.Parallel()

	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header()["Content-Type"] = nil

		_, _ = w.Write(png)
	}))
	t.Cleanup(ts.Close)

	client, server := thumbClient(t, ts)

	body, contentType, err := client.GetThumb(t.Context(), server, "/library/metadata/1/thumb/2")
	require.NoError(t, err)
	assert.Equal(t, "image/png", contentType)
	assert.Equal(t, png, body)
}

func TestGetThumbRefusesAnOversizedResponse(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")

		_, _ = w.Write(make([]byte, maxThumbBytes+1))
	}))
	t.Cleanup(ts.Close)

	client, server := thumbClient(t, ts)

	_, _, err := client.GetThumb(t.Context(), server, "/library/metadata/1/thumb/2")
	require.Error(t, err, "a thumbnail over the cap is never buffered whole")
}

func TestNewClientSharesItsConnectionPool(t *testing.T) {
	t.Parallel()

	first := NewClient(ClientConfig{Token: "one"})
	second := NewClient(ClientConfig{Token: "two"})

	assert.Same(t, first.httpClient, second.httpClient, "API requests reuse one pool")
	assert.Same(t, first.thumbClient, second.thumbClient, "thumbnails reuse one pool")

	custom := NewClient(ClientConfig{Timeout: time.Second})
	assert.NotSame(t, first.httpClient, custom.httpClient, "a custom timeout gets its own pool")
}

// thumbClient builds a Plex client aimed at a stand-in media server.
//
// Parameters:
//   - t: The test the client belongs to.
//   - ts: The media server stand-in.
//
// Returns:
//   - client: A Plex client.
//   - server: The server the stand-in answers as.
func thumbClient(t *testing.T, ts *httptest.Server) (*Client, Server) {
	t.Helper()

	host, port := extractAddrPort(t, ts)

	return NewClient(
			ClientConfig{
				Product:  productName,
				ClientID: "test",
				Token:    "srv-token",
				Timeout:  5 * time.Second,
			},
		),
		Server{
			Name:    "Test",
			Address: host,
			Port:    port,
			Token:   "srv-token",
			Scheme:  httpScheme,
			Local:   false,
		}
}

func TestGetThumb(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/metadata/1/thumb/2", r.URL.Path)
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte("jpeg-bytes"))
	}))
	defer ts.Close()

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

	body, contentType, err := client.GetThumb(t.Context(), server, "/library/metadata/1/thumb/2")
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", contentType)
	assert.Equal(t, []byte("jpeg-bytes"), body)

	_, _, err = client.GetThumb(t.Context(), server, "/etc/passwd")
	require.ErrorIs(t, err, ErrInvalidThumbPath)
}
