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

// PreferUniqueServers keeps one connection per server, preferring local ones.
// Servers are told apart by machine id, or by name when discovery reported
// none.
//
// Parameters:
//   - servers: Discovered server connections, possibly including remotes.
//
// Returns:
//   - unique: Deduplicated servers with local connections preferred.
func PreferUniqueServers(servers []Server) []Server {
	byServer := make(map[string]Server, len(servers))
	order := make([]string, 0, len(servers))

	for _, server := range servers {
		identity := serverIdentity(server)

		existing, seen := byServer[identity]
		if !seen {
			byServer[identity] = server
			order = append(order, identity)

			continue
		}

		if server.Local && !existing.Local {
			byServer[identity] = server
		}
	}

	unique := make([]Server, 0, len(order))
	for _, identity := range order {
		unique = append(unique, byServer[identity])
	}

	return unique
}

// ConnectionsOf returns every connection of the server a connection belongs to.
//
// Parameters:
//   - servers: Discovered server connections.
//   - server: One connection of the server.
//
// Returns:
//   - connections: The server's connections, in discovery order.
func ConnectionsOf(servers []Server, server Server) []Server {
	identity := serverIdentity(server)

	connections := make([]Server, 0, len(servers))

	for _, candidate := range servers {
		if serverIdentity(candidate) == identity {
			connections = append(connections, candidate)
		}
	}

	return connections
}

// SelectionKey names one discovered connection without carrying its token, so
// a page can post it back and the choice can be looked up again.
//
// Parameters:
//   - server: A discovered connection.
//
// Returns:
//   - key: The server's machine id and the connection's origin.
func SelectionKey(server Server) string {
	return serverIdentity(server) + " " + server.Scheme + "://" +
		net.JoinHostPort(server.Address, strconv.Itoa(server.Port))
}

// serverIdentity names the server a connection belongs to.
//
// Parameters:
//   - server: A connection.
//
// Returns:
//   - identity: The machine id, or the name when discovery reported none.
func serverIdentity(server Server) string {
	if server.MachineID != "" {
		return server.MachineID
	}

	return server.Name
}

// ParseServerURL reads a Plex Media Server URL a user typed, without a token.
//
// Parameters:
//   - rawURL: An http or https URL, such as https://plex.example.com.
//
// Returns:
//   - server: The connection the URL names, with no token.
//   - ok: False unless the URL is http or https and names a host.
func ParseServerURL(rawURL string) (Server, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return EmptyServer(), false
	}

	if parsed.Scheme != httpScheme && parsed.Scheme != defaultScheme {
		return EmptyServer(), false
	}

	portNum, ok := portFromURL(parsed)
	if !ok {
		return EmptyServer(), false
	}

	return Server{
		Name:      parsed.Host,
		Address:   parsed.Hostname(),
		Port:      portNum,
		Token:     "",
		Scheme:    parsed.Scheme,
		Local:     false,
		MachineID: "",
		Relay:     false,
	}, true
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
