// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"net/url"
	"strconv"
)

const (
	// DefaultPlexPort is used when a PMS URL omits an explicit port.
	defaultPlexPort = 32400

	// HttpsPort is the default HTTPS port.
	httpsPort = 443

	// HttpPort is the default HTTP port.
	httpPort = 80
)

// PreferUniqueServers keeps one connection per server name, preferring local ones.
//
// Parameters:
//   - servers: Discovered server connections, possibly including remotes.
//
// Returns:
//   - unique: Deduplicated servers with local connections preferred.
func PreferUniqueServers(servers []Server) []Server {
	byName := make(map[string]Server, len(servers))
	order := make([]string, 0, len(servers))

	for _, server := range servers {
		existing, seen := byName[server.Name]
		if !seen {
			byName[server.Name] = server
			order = append(order, server.Name)

			continue
		}

		if server.Local && !existing.Local {
			byName[server.Name] = server
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
//   - rawURL: Plex Media Server base URL, such as http://192.168.1.5:32400.
//   - token: Plex access token.
//
// Returns:
//   - server: Parsed server connection.
//   - ok: False when the URL is empty or invalid.
func ServerFromURL(rawURL, token string) (Server, bool) {
	if rawURL == "" || token == "" {
		return EmptyServer(), false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return EmptyServer(), false
	}

	portNum, ok := portFromURL(parsed)
	if !ok {
		return EmptyServer(), false
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

// portFromURL reads an explicit port or the scheme default.
// DefaultPortForScheme returns the implicit port for a URL scheme.
func defaultPortForScheme(scheme string) int {
	switch scheme {
	case defaultScheme:
		return httpsPort
	case httpScheme:
		return httpPort
	default:
		return defaultPlexPort
	}
}

// portFromURL reads an explicit port or the scheme default.
func portFromURL(parsed *url.URL) (int, bool) {
	if parsed.Port() != "" {
		portNum, err := strconv.Atoi(parsed.Port())

		return portNum, err == nil
	}

	return defaultPortForScheme(parsed.Scheme), true
}
