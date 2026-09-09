// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"net/url"
	"strconv"
)

// Server represents a Plex server connection.
type Server struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
	Scheme  string `json:"scheme"`
	Local   bool   `json:"local"`
}

const (
	defaultPlexPort = 32400
	httpsPort       = 443
	httpPort        = 80
	defaultScheme   = "https"
	httpScheme      = "http"
)

// Empty returns a Server with every exported field set to its zero value.
//
// Returns:
//   - server: A Server with every exported field set to its zero value.
func Empty() Server {
	return Server{
		Name:    "",
		Address: "",
		Port:    0,
		Token:   "",
		Scheme:  "",
		Local:   false,
	}
}

// PreferUniqueServers keeps one connection per server name, preferring local
// ones.
//
// Parameters:
//   - servers: Servers.
//
// Returns:
//   - items: The items.
func PreferUniqueServers(servers []Server) []Server {
	byName := make(map[string]Server, len(servers))
	order := make([]string, 0, len(servers))

	for _, srv := range servers {
		existing, seen := byName[srv.Name]
		if !seen {
			byName[srv.Name] = srv
			order = append(order, srv.Name)

			continue
		}

		if srv.Local && !existing.Local {
			byName[srv.Name] = srv
		}
	}

	unique := make([]Server, 0, len(order))
	for _, name := range order {
		unique = append(unique, byName[name])
	}

	return unique
}

// ServerFromURL builds a Server from a base URL and access token.
//
// Parameters:
//   - rawURL: Raw url.
//   - token: Token.
//
// Returns:
//   - server: A Server from a base URL and access token.
//   - ok: True when the condition holds.
func ServerFromURL(rawURL, token string) (Server, bool) {
	if rawURL == "" || token == "" {
		return Empty(), false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return Empty(), false
	}

	portNum, ok := portFromURL(parsed)
	if !ok {
		return Empty(), false
	}

	scheme := parsed.Scheme
	if scheme == "" {
		scheme = httpScheme
	}

	return Server{
		Name:    parsed.Host,
		Address: parsed.Hostname(),
		Port:    portNum,
		Token:   token,
		Scheme:  scheme,
		Local:   true,
	}, true
}

// SameConnection reports whether two servers share scheme, host, and port.
//
// Parameters:
//   - left: Left.
//   - right: Right.
//
// Returns:
//   - ok: True when two servers share scheme, host, and port.
func SameConnection(left, right Server) bool {
	return left.Scheme == right.Scheme && left.Address == right.Address && left.Port == right.Port
}

// DefaultPortForScheme returns the default port for scheme.
//
// Parameters:
//   - scheme: Scheme.
//
// Returns:
//   - n: The default port for scheme.
func DefaultPortForScheme(scheme string) int {
	switch scheme {
	case defaultScheme:
		return httpsPort
	case httpScheme:
		return httpPort
	default:
		return defaultPlexPort
	}
}

// portFromURL returns the port from url.
//
// Parameters:
//   - parsed: Parsed.
//
// Returns:
//   - n: The port from url.
//   - ok: True when the condition holds.
func portFromURL(parsed *url.URL) (int, bool) {
	if parsed.Port() != "" {
		portNum, err := strconv.Atoi(parsed.Port())

		return portNum, err == nil
	}

	return DefaultPortForScheme(parsed.Scheme), true
}
