// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"context"
	"slices"
	"sync"
)

// Connection preference, best first.
const (
	// rankLocal is a connection on the local network.
	rankLocal = iota
	// rankRemoteHTTPS is a direct remote connection over HTTPS.
	rankRemoteHTTPS
	// rankRemote is any other direct remote connection.
	rankRemote
	// rankRelay is a connection through the Plex relay.
	rankRelay
)

// FirstReachable pings every connection at once and returns the most
// preferred one that answered: local, then direct HTTPS, then any other
// direct connection, then the relay. A connection that answers no faster than
// a less preferred one still wins.
//
// Parameters:
//   - ctx: Cancellation and deadline for the pings.
//   - connections: Connections of one server.
//
// Returns:
//   - server: The preferred reachable connection.
//   - ok: False when no connection answered.
func (client *Client) FirstReachable(ctx context.Context, connections []Server) (Server, bool) {
	answered := client.pingAll(ctx, connections)

	best, found := EmptyServer(), false

	for _, connection := range answered {
		if !found || connectionRank(connection) < connectionRank(best) {
			best, found = connection, true
		}
	}

	return best, found
}

// pingAll pings every connection at once.
//
// Parameters:
//   - ctx: Cancellation and deadline for the pings.
//   - connections: Connections to ping.
//
// Returns:
//   - answered: The connections that answered, in their original order.
func (client *Client) pingAll(ctx context.Context, connections []Server) []Server {
	order := make([]int, 0, len(connections))

	for index := range client.answering(ctx, connections) {
		order = append(order, index)
	}

	slices.Sort(order)

	answered := make([]Server, 0, len(order))

	for _, index := range order {
		answered = append(answered, connections[index])
	}

	return answered
}

// answering pings every connection at once and reports the index of each one
// that answered. The channel closes once every ping has finished.
//
// Parameters:
//   - ctx: Cancellation and deadline for the pings.
//   - connections: Connections to ping.
//
// Returns:
//   - indexes: Indexes of the connections that answered, in no set order.
func (client *Client) answering(ctx context.Context, connections []Server) <-chan int {
	indexes := make(chan int, len(connections))

	var group sync.WaitGroup

	for index, connection := range connections {
		group.Go(func() {
			if client.Ping(ctx, connection) == nil {
				indexes <- index
			}
		})
	}

	group.Wait()
	close(indexes)

	return indexes
}

// connectionRank orders a connection by preference.
//
// Parameters:
//   - server: The connection.
//
// Returns:
//   - rank: Lower is preferred.
func connectionRank(server Server) int {
	switch {
	case server.Local:
		return rankLocal
	case server.Relay:
		return rankRelay
	case server.Scheme == "https":
		return rankRemoteHTTPS
	default:
		return rankRemote
	}
}
