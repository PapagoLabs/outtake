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
	"regexp"
	"strconv"
	"time"

	fiberClient "github.com/gofiber/fiber/v3/client"

	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex/decode/plextv"
	"github.com/PapagoLabs/outtake/internal/plex/decode/pms"
)

const (
	// serverAPIBase is the PMS library API prefix.
	serverAPIBase = "/library"

	// headerPlexToken is the Plex token header name.
	headerPlexToken = "X-Plex-Token"

	// headerAccept is the HTTP Accept header name.
	headerAccept = "Accept"

	// acceptJSON is the JSON Accept value.
	acceptJSON = "application/json"

	// acceptXML is the Accept value for plex.tv endpoints this client decodes as XML.
	acceptXML = "application/xml"

	// scaleMsToS converts Plex millisecond timestamps to seconds.
	scaleMsToS = 1000.0

	// pingTimeout is the PMS ping timeout.
	pingTimeout = 5 * time.Second

	// httpScheme is the HTTP URL scheme.
	httpScheme = "http"
)

// ratingKeyPattern matches a Plex rating key, the numeric id of a media item.
var ratingKeyPattern = regexp.MustCompile(`^\d{1,12}$`)

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
//
// Parameters:
//   - address: Host name or IP address.
//   - scheme: URL scheme the host is reached over.
//   - port: Port to join, unless it is the scheme default.
//
// Returns:
//   - hostPort: The host, with a port appended only when it is non-default.
func formatHost(address, scheme string, port int) string {
	if (scheme == defaultScheme && port == httpsPort) ||
		(scheme == httpScheme && port == httpPort) {

		return address
	}

	return net.JoinHostPort(address, strconv.Itoa(port))
}

// GetLibraries fetches libraries from the Plex server.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to query.
//
// Returns:
//   - libraries: The server's library sections.
//   - err: Non-nil when the PMS request or decode fails.
func (client *Client) GetLibraries(ctx context.Context, server Server) ([]Library, error) {
	scheme := server.Scheme
	if scheme == "" {
		scheme = defaultScheme
	}

	hostPort := formatHost(server.Address, scheme, server.Port)
	reqURL := fmt.Sprintf("%s://%s%s/sections/all", scheme, hostPort, serverAPIBase)

	cfg := fiberClient.Config{Ctx: ctx, Header: jsonHeaders(server.Token)}

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
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to query.
//   - libraryID: Section key.
//
// Returns:
//   - items: Every item the library returned.
//   - err: Non-nil when the PMS request or decode fails.
func (client *Client) GetMedia(
	ctx context.Context,
	server Server,
	libraryID string,
) ([]MediaItem, error) {
	page, err := client.GetMediaPage(ctx, server, libraryID, 0, 0, "")
	if err != nil {
		return nil, fmt.Errorf("get media page: %w", err)
	}

	return page.Items, nil
}

// GetMediaPage fetches one page of media items from a library.
//
// Parameters:
//   - ctx: Request context.
//   - server: PMS to query.
//   - libraryID: Section key.
//   - start: Container offset.
//   - size: Page size, or 0 for the PMS default.
//   - sort: PMS sort value such as titleSort:asc, or empty.
//
// Returns:
//   - page: Items and total size for the requested window.
//   - err: Non-nil when the PMS request or decode fails.
func (client *Client) GetMediaPage(
	ctx context.Context,
	server Server,
	libraryID string,
	start, size int,
	sort string,
) (MediaPage, error) {
	path := serverAPIBase + "/sections/" + url.PathEscape(libraryID) + "/all"

	resp, err := client.getPMS(ctx, server, path, mediaListQuery(start, size, sort))
	if err != nil {
		return MediaPage{}, fmt.Errorf("get media: %w", err)
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return MediaPage{}, fmt.Errorf("get media: %w", decodeErr)
	}

	return mediaPage(container, start, size), nil
}

// GetFirstCharacters fetches title first-character buckets for a library.
//
// Parameters:
//   - ctx: Request context.
//   - server: PMS to query.
//   - libraryID: Section key.
//
// Returns:
//   - index: First-character buckets.
//   - err: Non-nil when the PMS request or decode fails.
func (client *Client) GetFirstCharacters(
	ctx context.Context,
	server Server,
	libraryID string,
) ([]LetterIndex, error) {
	index, err := client.GetSectionIndex(ctx, server, libraryID, "firstCharacter")
	if err != nil {
		return nil, fmt.Errorf("get firstCharacter: %w", err)
	}

	return index, nil
}

// GetYears fetches year buckets for a library section.
//
// Parameters:
//   - ctx: Request context.
//   - server: PMS to query.
//   - libraryID: Section key.
//
// Returns:
//   - index: Year buckets.
//   - err: Non-nil when the PMS request or decode fails.
func (client *Client) GetYears(
	ctx context.Context,
	server Server,
	libraryID string,
) ([]LetterIndex, error) {
	index, err := client.GetSectionIndex(ctx, server, libraryID, "year")
	if err != nil {
		return nil, fmt.Errorf("get year: %w", err)
	}

	return index, nil
}

// GetSectionIndex fetches directory buckets for a library facet.
//
// Parameters:
//   - ctx: Request context.
//   - server: PMS to query.
//   - libraryID: Section key.
//   - facet: Directory facet name.
//
// Returns:
//   - index: Directory buckets.
//   - err: Non-nil when the facet is unsupported or the PMS request fails.
func (client *Client) GetSectionIndex(
	ctx context.Context,
	server Server,
	libraryID, facet string,
) ([]LetterIndex, error) {
	switch facet {
	case "firstCharacter", "year":
	default:
		return nil, fmt.Errorf("%w %q", ErrUnsupportedSectionIndex, facet)
	}

	path := serverAPIBase + "/sections/" + url.PathEscape(libraryID) + "/" + facet

	resp, err := client.getPMS(ctx, server, path, "")
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", facet, err)
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return nil, fmt.Errorf("get %s: %w", facet, decodeErr)
	}

	return directoryIndexes(container.Directory), nil
}

// directoryIndexes maps PMS directory entries onto jump buckets.
//
// Parameters:
//   - sections: PMS Directory rows.
//
// Returns:
//   - index: Title and size for each directory.
func directoryIndexes(sections []pms.Section) []LetterIndex {
	index := make([]LetterIndex, 0, len(sections))
	for i := range sections {
		index = append(index, directoryIndex(sections[i]))
	}

	return index
}

// directoryIndex maps one PMS directory entry onto a jump bucket.
//
// Parameters:
//   - section: PMS Directory row.
//
// Returns:
//   - entry: Title and size for the directory.
func directoryIndex(section pms.Section) LetterIndex {
	title := section.Title
	if title == "" {
		title = section.Key
	}

	size := section.Size
	if size == 0 {
		size = section.LeafCount
	}

	return LetterIndex{
		Title: title,
		Size:  size,
	}
}

// GetMediaPath fetches the file path for a media item.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS holding the item.
//   - mediaID: Rating key of the item.
//
// Returns:
//   - path: The first on-disk part path among the item's metadata rows.
//   - err: ErrInvalidMediaID for an id that is not a rating key,
//     ErrNoFilePathFound when no row carries a file, or a request error.
func (client *Client) GetMediaPath(
	ctx context.Context,
	server Server,
	mediaID string,
) (string, error) {
	if !ratingKeyPattern.MatchString(mediaID) {
		return "", fmt.Errorf("%w: %q", ErrInvalidMediaID, mediaID)
	}

	scheme := server.Scheme
	if scheme == "" {
		scheme = defaultScheme
	}

	hostPort := formatHost(server.Address, scheme, server.Port)
	reqURL := fmt.Sprintf("%s://%s/library/metadata/%s", scheme, hostPort, url.PathEscape(mediaID))

	cfg := fiberClient.Config{Ctx: ctx, Header: jsonHeaders(server.Token)}

	resp, err := client.httpClient.Get(reqURL, cfg)
	if err != nil {
		return "", fmt.Errorf("get media path: %w", err)
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return "", fmt.Errorf("decode media detail: %w", decodeErr)
	}

	for index := range container.Metadata {
		if file := firstMediaFile(&container.Metadata[index]); file != "" {
			return file, nil
		}
	}

	return "", ErrNoFilePathFound
}

// DiscoverServers discovers Plex servers.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//
// Returns:
//   - servers: One entry per connection the account exposes.
//   - err: Non-nil when the plex.tv request or decode fails.
func (client *Client) DiscoverServers(ctx context.Context) ([]Server, error) {
	resp, err := client.requestPlex(ctx, "/api/resources", "includeHttps=1", acceptXML)
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
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to ping.
//
// Returns:
//   - err: ErrServerReturnedError for a non-200 status, or a request error.
func (client *Client) Ping(ctx context.Context, server Server) error {
	scheme := server.Scheme
	if scheme == "" {
		scheme = defaultScheme
	}

	hostPort := formatHost(server.Address, scheme, server.Port)
	reqURL := fmt.Sprintf("%s://%s/identity", scheme, hostPort)

	cfg := fiberClient.Config{
		Ctx:    ctx,
		Header: map[string]string{headerPlexToken: server.Token},
	}

	cfg.Timeout = pingTimeout

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
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to query.
//
// Returns:
//   - identity: The PMS machine identifier and version.
//   - err: Non-nil when the PMS request or decode fails.
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

	cfg := fiberClient.Config{Ctx: ctx, Header: jsonHeaders(server.Token)}

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
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - query: Free-text search query.
//
// Returns:
//   - items: The plex.tv search hits.
//   - err: Non-nil when the plex.tv request or decode fails.
func (client *Client) SearchMedia(ctx context.Context, query string) ([]MediaItem, error) {
	resp, err := client.requestPlex(
		ctx,
		"/search",
		"query="+url.QueryEscape(query),
		acceptXML,
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
//   - ctx: Cancellation and deadline for the request.
//
// Returns:
//   - sessions: The plex.tv playback sessions.
//   - err: Non-nil when the plex.tv request or decode fails.
func (client *Client) GetSessions(ctx context.Context) ([]Session, error) {
	resp, err := client.requestPlex(ctx, "/status/sessions", "", acceptXML)
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
//
// Parameters:
//   - plexType: The Plex metadata type.
//
// Returns:
//   - mediaType: The outtake type, or "unknown" for an unmapped value.
func MapPlexType(plexType string) string {
	mapped, ok := plexTypeNames[plexType]
	if !ok {
		return "unknown"
	}

	return mapped
}

// serversFromDevices flattens discovered devices into server connections.
//
// Parameters:
//   - devices: Devices returned by /api/resources.
//
// Returns:
//   - servers: One entry per device connection.
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
//
// Parameters:
//   - name: Device name reported alongside the connection.
//   - token: Device access token.
//   - conn: The connection element.
//
// Returns:
//   - server: The parsed connection, or a connection built from its fields when
//     the URI is absent or unparseable.
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
//
// Parameters:
//   - token: Plex access token sent with the request.
//
// Returns:
//   - headers: The request headers.
func jsonHeaders(token string) map[string]string {
	return map[string]string{
		headerPlexToken: token,
		headerAccept:    acceptJSON,
	}
}

// sectionThumb prefers a section thumb, then the composite image.
//
// Parameters:
//   - section: PMS Directory row for the library section.
//
// Returns:
//   - thumbPath: The section thumb, or the composite image when there is none.
func sectionThumb(section pms.Section) string {
	if section.Thumb != "" {
		return section.Thumb
	}

	return section.Composite
}

// containerQuery builds PMS pagination query parameters.
//
// Parameters:
//   - start: Container offset.
//   - size: Page size, or 0 to omit pagination.
//
// Returns:
//   - query: Encoded query string, or empty when size is unset.
func containerQuery(start, size int) string {
	return mediaListQuery(start, size, "")
}

// mediaListQuery builds PMS pagination and sort query parameters.
//
// Parameters:
//   - start: Container offset.
//   - size: Page size, or 0 to omit pagination.
//   - sort: PMS sort value, or empty.
//
// Returns:
//   - query: Encoded query string, or empty when both size and sort are unset.
func mediaListQuery(start, size int, sort string) string {
	values := url.Values{}
	if size > 0 {
		if start < 0 {
			start = 0
		}

		values.Set("X-Plex-Container-Start", strconv.Itoa(start))
		values.Set("X-Plex-Container-Size", strconv.Itoa(size))
	}

	if sort != "" {
		values.Set("sort", sort)
	}

	return values.Encode()
}

// mediaPage maps a PMS container onto a page of media items.
//
// Parameters:
//   - container: Decoded PMS container.
//   - start: Container offset the window was requested at.
//   - size: Page size that was requested.
//
// Returns:
//   - page: Items with the total size to display.
func mediaPage(container pms.Container, start, size int) MediaPage {
	if start < 0 {
		start = 0
	}

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
//
// Parameters:
//   - entry: plex.tv media listing entry.
//   - libraryTitle: Owning library title, or empty.
//
// Returns:
//   - item: The mapped media item.
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
