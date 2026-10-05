// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"math"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex/decode/plextv"
)

func TestPreferUniqueServers(t *testing.T) {
	t.Parallel()

	servers := []Server{
		{Name: "Lounge", Address: "1.2.3.4", Port: 443, Token: "", Scheme: "", Local: false},
		{Name: "Lounge", Address: "192.168.1.5", Port: 32400, Token: "", Scheme: "", Local: true},
		{Name: "Office", Address: "10.0.0.2", Port: 32400, Token: "", Scheme: "", Local: true},
	}

	got := PreferUniqueServers(servers)
	require.Len(t, got, 2)
	assert.Equal(t, "192.168.1.5", got[0].Address)
	assert.Equal(t, "Lounge", got[0].Name)
	assert.Equal(t, "Office", got[1].Name)
}

func TestServersFromDevices_AccessTokenAndURI(t *testing.T) {
	t.Parallel()

	devices := []plextv.Device{
		{
			Name:        "Home",
			Address:     "",
			Port:        0,
			AccessToken: "camel-token",
			Connection: []plextv.Connection{
				{
					URI:      "https://plex.example.com",
					Address:  "plex.example.com",
					Port:     443,
					Protocol: defaultScheme,
					Local:    0,
				},
				{
					URI:      "https://192.168.120.103:32400",
					Address:  "192.168.120.103",
					Port:     32400,
					Protocol: defaultScheme,
					Local:    1,
				},
			},
		},
	}

	got := serversFromDevices(devices)
	require.Len(t, got, 2)
	assert.Equal(t, "plex.example.com", got[0].Address)
	assert.Equal(t, 443, got[0].Port)
	assert.Equal(t, defaultScheme, got[0].Scheme)
	assert.Equal(t, "camel-token", got[0].Token)
	assert.False(t, got[0].Local)
	assert.Equal(t, "192.168.120.103", got[1].Address)
	assert.True(t, got[1].Local)
}

func TestServerFromURL(t *testing.T) {
	t.Parallel()

	server, ok := ServerFromURL("http://192.168.1.9:32400", "tok")
	require.True(t, ok)
	assert.Equal(t, "192.168.1.9", server.Address)
	assert.Equal(t, 32400, server.Port)
	assert.Equal(t, "http", server.Scheme)
	assert.Equal(t, "tok", server.Token)

	_, ok = ServerFromURL("", "tok")
	assert.False(t, ok)

	_, ok = ServerFromURL("http://plex.local", "")
	assert.False(t, ok)
}

func TestServerFromURL_RejectedInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rawURL string
		token  string
	}{
		{name: "unparseable url", rawURL: "http://plex.local:notaport", token: "tok"},
		{name: "url without a host", rawURL: "/library/sections/all", token: "tok"},
		{
			name:   "port beyond int64",
			rawURL: "http://plex.local:99999999999999999999",
			token:  "tok",
		},
		{name: "missing token", rawURL: "http://192.168.1.9:32400", token: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server, ok := ServerFromURL(test.rawURL, test.token)
			assert.False(t, ok)
			assert.Equal(t, EmptyServer(), server)
		})
	}
}

func TestServerFromURL_DefaultsSchemeToHTTP(t *testing.T) {
	t.Parallel()

	server, ok := ServerFromURL("//plex.local", "tok")
	require.True(t, ok)
	assert.Equal(t, httpScheme, server.Scheme)
	assert.Equal(t, "plex.local", server.Address)
	assert.Equal(t, "plex.local", server.Name)
	assert.Equal(t, defaultPlexPort, server.Port)
}

func TestServerFromParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		scheme    string
		address   string
		port      string
		token     string
		wantOK    bool
		wantName  string
		wantPort  int
		wantIsTLS bool
	}{
		{
			name:     "blank scheme defaults to http",
			address:  "192.168.1.5",
			port:     "32400",
			token:    "tok",
			wantOK:   true,
			wantName: "192.168.1.5:32400",
			wantPort: 32400,
		},
		{
			name:      "https keeps the explicit port",
			scheme:    defaultScheme,
			address:   "plex.example.com",
			port:      "32400",
			token:     "tok",
			wantOK:    true,
			wantName:  "plex.example.com:32400",
			wantPort:  32400,
			wantIsTLS: true,
		},
		{
			name:     "blank port over http uses port 80",
			scheme:   httpScheme,
			address:  "plex.local",
			token:    "tok",
			wantOK:   true,
			wantName: "plex.local:80",
			wantPort: httpPort,
		},
		{
			name:      "blank port over https uses port 443",
			scheme:    defaultScheme,
			address:   "plex.local",
			token:     "tok",
			wantOK:    true,
			wantName:  "plex.local:443",
			wantPort:  httpsPort,
			wantIsTLS: true,
		},
		{
			name:      "zero port uses the scheme default",
			scheme:    defaultScheme,
			address:   "plex.local",
			port:      "0",
			token:     "tok",
			wantOK:    true,
			wantName:  "plex.local:443",
			wantPort:  httpsPort,
			wantIsTLS: true,
		},
		{
			name:     "unrecognized scheme uses the plex port",
			scheme:   "ftp",
			address:  "plex.local",
			token:    "tok",
			wantOK:   true,
			wantName: "plex.local:32400",
			wantPort: defaultPlexPort,
		},
		{
			name:     "ipv6 address is bracketed in the name",
			scheme:   httpScheme,
			address:  "::1",
			port:     "32400",
			token:    "tok",
			wantOK:   true,
			wantName: "[::1]:32400",
			wantPort: 32400,
		},
		{
			name:     "port is parsed as a number",
			scheme:   httpScheme,
			address:  "plex.local",
			port:     "8080",
			token:    "tok",
			wantOK:   true,
			wantName: "plex.local:8080",
			wantPort: 8080,
		},
		{name: "missing address is rejected", scheme: httpScheme, port: "32400", token: "tok"},
		{
			name:    "missing token is rejected",
			scheme:  httpScheme,
			address: "192.168.1.5",
			port:    "32400",
		},
		{
			name:    "non-numeric port is rejected",
			scheme:  httpScheme,
			address: "192.168.1.5",
			port:    "not-a-port",
			token:   "tok",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server, ok := ServerFromParts(
				test.scheme,
				test.address,
				test.port,
				test.token,
			)

			assert.Equal(t, test.wantOK, ok)

			if !test.wantOK {
				assert.Equal(t, EmptyServer(), server)

				return
			}

			assert.Equal(t, test.wantName, server.Name)
			assert.Equal(t, test.address, server.Address)
			assert.Equal(t, test.wantPort, server.Port)
			assert.Equal(t, test.token, server.Token)
			assert.True(t, server.Local)
			assert.Equal(t, test.wantIsTLS, server.Scheme == defaultScheme)
		})
	}
}

func TestServerFromParts_DefaultsBlankScheme(t *testing.T) {
	t.Parallel()

	server, ok := ServerFromParts("", "192.168.1.5", "32400", "tok")
	require.True(t, ok)
	assert.Equal(t, httpScheme, server.Scheme)
}

func TestDefaultPortForScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scheme string
		want   int
	}{
		{name: "https scheme", scheme: defaultScheme, want: httpsPort},
		{name: "http scheme", scheme: httpScheme, want: httpPort},
		{name: "ftp scheme", scheme: "ftp", want: defaultPlexPort},
		{name: "empty scheme", scheme: "", want: defaultPlexPort},
		{name: "mixed case scheme", scheme: "HTTPS", want: defaultPlexPort},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, defaultPortForScheme(test.scheme))
		})
	}
}

func TestPortFromURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rawURL string
		want   int
		wantOK bool
	}{
		{name: "explicit port", rawURL: "http://plex.local:32400", want: 32400, wantOK: true},
		{name: "http default", rawURL: "http://plex.local", want: httpPort, wantOK: true},
		{name: "https default", rawURL: "https://plex.local", want: httpsPort, wantOK: true},
		{name: "no scheme", rawURL: "//plex.local", want: defaultPlexPort, wantOK: true},
		{
			name:   "port beyond int64",
			rawURL: "http://plex.local:99999999999999999999",
			want:   math.MaxInt64,
			wantOK: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := url.Parse(test.rawURL)
			require.NoError(t, err)

			port, ok := portFromURL(parsed)
			assert.Equal(t, test.wantOK, ok)
			assert.Equal(t, test.want, port)
		})
	}
}
