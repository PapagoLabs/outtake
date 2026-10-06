// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"
)

func TestHostPolicyAllows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		allowed []string
		host    string
		want    bool
	}{
		{name: "IPv4 literal", host: "192.168.1.20", want: true},
		{name: "IPv6 literal", host: "[::1]", want: true},
		{name: "bare IPv6 literal", host: "fe80::1", want: true},
		{name: "localhost", host: "localhost", want: true},
		{name: "localhost subdomain", host: "outtake.localhost", want: true},
		{name: "single-label name", host: "nas", want: true},
		{name: "mDNS name", host: "nas.local", want: true},
		{name: "home network name", host: "outtake.home.arpa", want: true},
		{name: "internal name", host: "outtake.svc.internal", want: true},
		{name: "letter case is ignored", host: "NAS.LAN", want: true},
		{name: "public name", host: "rebind.attacker.example", want: false},
		{name: "empty host", host: "", want: false},
		{
			name:    "listed name",
			allowed: []string{"clips.example.com"},
			host:    "clips.example.com",
			want:    true,
		},
		{
			name:    "listed name with a trailing dot",
			allowed: []string{"clips.example.com"},
			host:    "clips.example.com.",
			want:    true,
		},
		{
			name:    "listed name only",
			allowed: []string{"clips.example.com"},
			host:    "evil.example.com",
			want:    false,
		},
		{
			name:    "listed domain",
			allowed: []string{".example.com"},
			host:    "clips.example.com",
			want:    true,
		},
		{
			name:    "listed domain apex",
			allowed: []string{".example.com"},
			host:    "example.com",
			want:    true,
		},
		{
			name:    "listed domain only",
			allowed: []string{".example.com"},
			host:    "example.com.evil.test",
			want:    false,
		},
		{
			name:    "listed entry case",
			allowed: []string{" Clips.Example.COM "},
			host:    "clips.example.com",
			want:    true,
		},
		{name: "wildcard", allowed: []string{"*"}, host: "anything.example", want: true},
		{name: "blank entries", allowed: []string{"", "  "}, host: "anything.example", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, newHostPolicy(test.allowed).allows(test.host))
		})
	}
}

func TestHostGuardPassesAnAllowedHost(t *testing.T) {
	t.Parallel()

	app := chainApp(t, okHandler(), HostGuard([]string{"clips.example.com"}))

	got := requestHost(t, app, "/", "clips.example.com:8080")

	assert.Equal(t, fiber.StatusOK, got.status)
}

func TestHostGuardRefusesAnUnlistedHost(t *testing.T) {
	t.Parallel()

	app := chainApp(t, writeHandler("reached"), HostGuard([]string{"clips.example.com"}))

	got := requestHost(t, app, "/", "rebind.attacker.example")

	assert.Equal(t, fiber.StatusMisdirectedRequest, got.status)
	assert.Contains(t, got.body, `"rebind.attacker.example"`)
	assert.Contains(t, got.body, "OUTTAKE_ALLOWED_HOSTS")
	assert.NotContains(t, got.body, "reached", "the refused request never reaches the handler")
}

func TestHostGuardLetsAnExemptPathThrough(t *testing.T) {
	t.Parallel()

	app := chainApp(t, okHandler(), HostGuard(nil, "/api/healthz"))

	assert.Equal(t, fiber.StatusOK, requestHost(t, app, "/api/healthz", "probe.example").status)
	assert.Equal(t,
		fiber.StatusMisdirectedRequest,
		requestHost(t, app, "/api/clips", "probe.example").status,
		"only the exempt path skips the check",
	)
}

// requestHost sends a request carrying a given Host header.
//
// Parameters:
//   - t: The test that issues the request.
//   - app: The Fiber application serving the middleware chain.
//   - target: Request path.
//   - host: Host header value, which may include a port.
//
// Returns:
//   - got: The response the application produced.
func requestHost(t *testing.T, app *fiber.App, target, host string) answer {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)

	req.Host = host

	resp, err := app.Test(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return answer{
		status:   resp.StatusCode,
		location: resp.Header.Get(fiber.HeaderLocation),
		body:     string(body),
	}
}
