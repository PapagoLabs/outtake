// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"net"
	"net/url"
	"strconv"
)

const (
	// defaultPlexPort is used when a PMS URL omits an explicit port.
	defaultPlexPort = 32400

	// httpsPort is the default HTTPS port.
	httpsPort = 443

	// httpPort is the default HTTP port.
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

// ServerFromParts builds a Server from the parts of a base URL.
//
// Parameters:
//   - scheme: URL scheme, defaulting to http when blank.
//   - address: Server host, which is required.
//   - port: Server port, where blank or zero means the scheme's own default.
//   - token: Plex access token.
//
// Returns:
//   - server: Assembled server connection.
//   - ok: False when the host or token is missing, or the port is not a number.
func ServerFromParts(scheme, address, port, token string) (Server, bool) {
	if address == "" || token == "" {
		return EmptyServer(), false
	}

	if scheme == "" {
		scheme = httpScheme
	}

	portNum := defaultPortForScheme(scheme)

	if port != "" && port != "0" {
		parsed, err := strconv.Atoi(port)
		if err != nil {
			return EmptyServer(), false
		}

		portNum = parsed
	}

	return Server{
		Name:    net.JoinHostPort(address, strconv.Itoa(portNum)),
		Address: address,
		Port:    portNum,
		Token:   token,
		Scheme:  scheme,
		Local:   true,
	}, true
}

// defaultPortForScheme returns the implicit port for a URL scheme.
//
// Parameters:
//   - scheme: URL scheme, or empty for an unrecognized one.
//
// Returns:
//   - port: The scheme's implicit port, or defaultPlexPort for anything else.
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
//
// Parameters:
//   - parsed: Parsed PMS URL.
//
// Returns:
//   - port: The explicit port, or the scheme's implicit port.
//   - ok: False when an explicit port is present but not a number.
func portFromURL(parsed *url.URL) (int, bool) {
	if parsed.Port() != "" {
		portNum, err := strconv.Atoi(parsed.Port())

		return portNum, err == nil
	}

	return defaultPortForScheme(parsed.Scheme), true
}
