// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"context"
	"fmt"
	"net/url"

	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex/decode/plextv"
	plexserver "github.com/PapagoLabs/outtake/internal/plex/server"
)

// DiscoverServers discovers Plex servers.
//
// Parameters:
//   - ctx: Cancellation context.
//
// Returns:
//   - items: The items.
//   - err: The error, if any.
func (client *Client) DiscoverServers(ctx context.Context) ([]Server, error) {
	resp, err := client.doRequest(ctx, "/api/resources", "includeHttps=1")
	if err != nil {
		return nil, fmt.Errorf("discover servers: %w", err)
	}

	body := resp.Body()
	if len(body) == 0 {
		return nil, errEmptyBody
	}

	devices, err := plextv.Devices(body)
	if err != nil {
		return nil, fmt.Errorf("decode servers: %w", err)
	}

	servers := serversFromDevices(devices)

	logging.Logger.Debug().
		Int("count", len(servers)).
		Msg("discovered servers")

	return servers, nil
}

// SearchMedia searches for media across all accessible servers using the
// Plex.tv API.
//
// Parameters:
//   - ctx: Cancellation context.
//   - query: Query.
//
// Returns:
//   - items: The items.
//   - err: The error, if any.
func (client *Client) SearchMedia(ctx context.Context, query string) ([]MediaItem, error) {
	resp, err := client.doRequest(
		ctx,
		"/search",
		"query="+url.QueryEscape(query),
	)
	if err != nil {
		return nil, fmt.Errorf("search media: %w", err)
	}

	body := resp.Body()
	if len(body) == 0 {
		return nil, errEmptyBody
	}

	videos, _, err := plextv.Search(body)
	if err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	items := make([]MediaItem, 0, len(videos))
	for _, entry := range videos {
		items = append(items, mediaItemFromEntry(entry, ""))
	}

	return items, nil
}

// GetSessions fetches active sessions using the Plex.tv API.
//
// Parameters:
//   - ctx: Cancellation context.
//
// Returns:
//   - items: The active sessions using the Plex.tv API.
//   - err: The error, if any.
func (client *Client) GetSessions(ctx context.Context) ([]Session, error) {
	resp, err := client.doRequest(ctx, "/status/sessions", "")
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}

	body := resp.Body()
	if len(body) == 0 {
		return nil, nil
	}

	entries, err := plextv.Sessions(body)
	if err != nil {
		return nil, fmt.Errorf("decode sessions: %w", err)
	}

	sessions := make([]Session, 0, len(entries))
	for _, entry := range entries {
		sessions = append(sessions, Session{
			ID:         entry.PlaybackID(),
			MediaItem:  mediaItemFromEntry(entry.Media(), ""),
			Title:      entry.Title,
			Duration:   float64(entry.Duration) / scaleMsToS,
			ViewOffset: float64(entry.ViewOffset) / scaleMsToS,
		})
	}

	return sessions, nil
}

// serversFromDevices flattens discovered devices into server connections.
//
// Parameters:
//   - devices: Devices.
//
// Returns:
//   - items: The items.
func serversFromDevices(devices []plextv.Device) []Server {
	servers := make([]Server, 0, len(devices))

	for _, device := range devices {
		token := device.AccessToken

		for _, conn := range device.Connection {
			servers = append(servers, serverFromConnection(device.Name, token, conn))
		}
	}

	return servers
}

// mediaItemFromEntry converts a Plex media listing entry.
//
// Parameters:
//   - entry: Entry.
//   - libraryTitle: Library title.
//
// Returns:
//   - mediaItem: The media item.
func mediaItemFromEntry(entry plextv.Media, libraryTitle string) MediaItem {
	return MediaItem{
		ID:           entry.ID(),
		Title:        entry.Title,
		Type:         MapPlexType(entry.Type),
		Duration:     float64(entry.Duration) / scaleMsToS,
		ThumbPath:    entry.Thumb,
		LibraryTitle: libraryTitle,
	}
}

// serverFromConnection maps a Plex Connection element onto a Server.
//
// Parameters:
//   - name: Name.
//   - token: Token.
//   - conn: Conn.
//
// Returns:
//   - server: A Plex Connection element onto a Server.
func serverFromConnection(name, token string, conn plextv.Connection) Server {
	if conn.URI != "" {
		parsed, ok := plexserver.ServerFromURL(conn.URI, token)
		if ok {
			parsed.Name = name
			parsed.Local = conn.Local == 1

			return parsed
		}
	}

	scheme := conn.Protocol
	if scheme == "" {
		scheme = defaultScheme
	}

	port := conn.Port
	if port == 0 {
		port = plexserver.DefaultPortForScheme(scheme)
	}

	return Server{
		Name:    name,
		Address: conn.Address,
		Port:    port,
		Token:   token,
		Scheme:  scheme,
		Local:   conn.Local == 1,
	}
}
