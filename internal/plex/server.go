// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package plex provides a client for the Plex Media Server API.
package plex

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex/decode/plextv"
	"github.com/PapagoLabs/outtake/internal/plex/decode/pms"
)

const (
	// ServerAPIBase is the PMS library API prefix.
	serverAPIBase = "/library"

	// HeaderPlexToken is the Plex token header name.
	headerPlexToken = "X-Plex-Token"

	// HeaderAccept is the HTTP Accept header name.
	headerAccept = "Accept"

	// AcceptJSON is the JSON Accept value.
	acceptJSON = "application/json"

	// ScaleMsToS converts Plex millisecond timestamps to seconds.
	scaleMsToS = 1000.0

	// PingTimeoutSec is the PMS ping timeout in seconds.
	pingTimeoutSec = 5

	// HttpScheme is the HTTP URL scheme.
	httpScheme = "http"
)

// plexTypeNames maps Plex metadata types onto outtake type names.
var plexTypeNames = map[string]string{
	"movie":    "movie",
	TypeShow:   TypeShow,
	"episode":  "episode",
	TypeSeason: TypeSeason,
	TypeAlbum:  TypeAlbum,
	"track":    "track",
	TypeArtist: TypeArtist,
	"photo":    "photo",
	"clip":     "clip",
}

// formatHost returns host:port, omitting the port if it's the default for the scheme.
func formatHost(address, scheme string, port int) string {
	if (scheme == defaultScheme && port == httpsPort) ||
		(scheme == httpScheme && port == httpPort) {

		return address
	}

	return net.JoinHostPort(address, strconv.Itoa(port))
}

// GetLibraries fetches libraries from the Plex server.
func (client *Client) GetLibraries(ctx context.Context, server Server) ([]Library, error) {
	scheme := server.Scheme
	if scheme == "" {
		scheme = defaultScheme
	}

	hostPort := formatHost(server.Address, scheme, server.Port)
	reqURL := fmt.Sprintf("%s://%s%s/sections/all", scheme, hostPort, serverAPIBase)

	cfg := newRequestConfig(ctx, jsonHeaders(server.Token), nil)

	resp, err := client.httpClient.Get(reqURL, cfg)
	if err != nil {
		return nil, fmt.Errorf("get libraries: %w", err)
	}

	container, err := decodePMS(resp.Body())
	if err != nil {
		return nil, fmt.Errorf("decode libraries: %w", err)
	}

	libs := make([]Library, 0, len(container.Directory))
	for _, section := range container.Directory {
		libs = append(libs, Library{
			ID:        section.Key,
			Title:     section.Title,
			Type:      section.Type,
			ThumbPath: sectionThumb(section),
		})
	}

	logging.Logger.Debug().
		Str("server", server.Name).
		Int("count", len(libs)).
		Msg("fetched libraries")

	return libs, nil
}

// GetMedia fetches media items from a library.
func (client *Client) GetMedia(
	ctx context.Context,
	server Server,
	libraryID string,
) ([]MediaItem, error) {
	page, err := client.GetMediaPage(ctx, server, libraryID, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("get media page: %w", err)
	}

	return page.Items, nil
}

// GetMediaPage fetches one page of media items from a library.
func (client *Client) GetMediaPage(
	ctx context.Context,
	server Server,
	libraryID string,
	start, size int,
) (MediaPage, error) {
	path := serverAPIBase + "/sections/" + url.PathEscape(libraryID) + "/all"

	resp, err := client.getPMS(ctx, server, path, containerQuery(start, size))
	if err != nil {
		return MediaPage{}, fmt.Errorf("get media: %w", err)
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return MediaPage{}, fmt.Errorf("get media: %w", decodeErr)
	}

	return mediaPage(container, start, size), nil
}

// GetMediaPath fetches the file path for a media item.
func (client *Client) GetMediaPath(
	ctx context.Context,
	server Server,
	mediaID string,
) (string, error) {
	// Resolve the filesystem path for a media item.
	scheme := server.Scheme
	if scheme == "" {
		scheme = defaultScheme
	}

	hostPort := formatHost(server.Address, scheme, server.Port)
	reqURL := fmt.Sprintf("%s://%s/library/metadata/%s", scheme, hostPort, mediaID)

	cfg := newRequestConfig(ctx, jsonHeaders(server.Token), nil)

	resp, err := client.httpClient.Get(reqURL, cfg)
	if err != nil {
		return "", fmt.Errorf("get media path: %w", err)
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return "", fmt.Errorf("decode media detail: %w", decodeErr)
	}

	for index := range container.Metadata {
		if file := firstMediaFile(container.Metadata[index]); file != "" {
			return file, nil
		}
	}

	return "", ErrNoFilePathFound
}

// DiscoverServers discovers Plex servers.
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

// Ping pings the server to check connectivity.
func (client *Client) Ping(ctx context.Context, server Server) error {
	scheme := server.Scheme
	if scheme == "" {
		scheme = defaultScheme
	}

	hostPort := formatHost(server.Address, scheme, server.Port)
	reqURL := fmt.Sprintf("%s://%s/identity", scheme, hostPort)

	cfg := newRequestConfig(ctx, map[string]string{headerPlexToken: server.Token}, nil)

	cfg.Timeout = pingTimeoutSec * time.Second

	resp, err := client.httpClient.Get(reqURL, cfg)
	if err != nil {
		return fmt.Errorf("ping server: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("%w %d", ErrServerReturnedError, resp.StatusCode())
	}

	return nil
}

// GetServerIdentity fetches the server identity.
func (client *Client) GetServerIdentity(
	ctx context.Context,
	server Server,
) (*ServerIdentity, error) {
	// Fetch the PMS machine identity.
	scheme := server.Scheme
	if scheme == "" {
		scheme = defaultScheme
	}

	hostPort := formatHost(server.Address, scheme, server.Port)
	reqURL := fmt.Sprintf("%s://%s/identity", scheme, hostPort)

	cfg := newRequestConfig(ctx, jsonHeaders(server.Token), nil)

	resp, err := client.httpClient.Get(reqURL, cfg)
	if err != nil {
		return nil, fmt.Errorf("get server identity: %w", err)
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return nil, fmt.Errorf("decode server identity: %w", decodeErr)
	}

	return &ServerIdentity{
		MachineIdentifier: container.MachineIdentifier,
		Version:           container.Version,
	}, nil
}

// SearchMedia searches for media across all accessible servers using the Plex.tv API.
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

// MapPlexType maps Plex type strings to standardized types.
func MapPlexType(plexType string) string {
	mapped, ok := plexTypeNames[plexType]
	if !ok {
		return "unknown"
	}

	return mapped
}

// serversFromDevices flattens discovered devices into server connections.
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

// serverFromConnection maps a Plex Connection element onto a Server.
func serverFromConnection(name, token string, conn plextv.Connection) Server {
	if conn.URI != "" {
		parsed, ok := ServerFromURL(conn.URI, token)
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
		port = defaultPortForScheme(scheme)
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

// jsonHeaders returns PMS JSON request headers as documented by the OpenAPI spec.
func jsonHeaders(token string) map[string]string {
	return map[string]string{
		headerPlexToken: token,
		headerAccept:    acceptJSON,
	}
}

// sectionThumb prefers a section thumb, then the composite image.
func sectionThumb(section pms.Section) string {
	if section.Thumb != "" {
		return section.Thumb
	}

	return section.Composite
}

// containerQuery builds PMS pagination query parameters.
func containerQuery(start, size int) string {
	if size <= 0 {
		return ""
	}

	if start < 0 {
		start = 0
	}

	return "X-Plex-Container-Start=" + strconv.Itoa(start) +
		"&X-Plex-Container-Size=" + strconv.Itoa(size)
}

// mediaPage maps a PMS container onto a page of media items.
func mediaPage(container pms.Container, start, size int) MediaPage {
	items := metadataItems(container.Metadata, "")
	total := container.TotalSize
	if total == 0 {
		total = container.Size
	}

	if total == 0 {
		total = start + len(items)
	}

	return MediaPage{
		Items: items,
		Total: total,
		Start: start,
		Size:  size,
	}
}

// mediaItemFromEntry converts a Plex media listing entry.
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
