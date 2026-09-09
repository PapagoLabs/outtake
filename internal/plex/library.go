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

	// scaleMsToS converts Plex millisecond timestamps to seconds.
	scaleMsToS = 1000.0

	// pingTimeoutSec is the PMS ping timeout in seconds.
	pingTimeoutSec = 5

	// httpScheme is the HTTP URL scheme.
	httpScheme = "http"

	httpsPort = 443
	httpPort  = 80
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

// formatHost returns host:port, omitting the port if it's the default for the
// scheme.
//
// Parameters:
//   - address: Host:port or host for the Plex server.
//   - scheme: URL scheme (http or https).
//   - port: TCP port; 0 means scheme default.
//
// Returns:
//   - value: The host:port, omitting the port if it's the default for the.
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
//   - ctx: Cancels or deadlines this call.
//   - server: Plex Media Server connection (URL and token).
//
// Returns:
//   - items: The libraries from the Plex server.
//   - err: Wrapped failure such as "get libraries"; "decode libraries".
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
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//   - server: Plex Media Server connection (URL and token).
//   - libraryID: Typed string argument for GetMedia.
//
// Returns:
//   - items: The media items from a library.
//   - err: Wrapped failure from "get media page".
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
//   - ctx: Cancels or deadlines this call.
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
//   - ctx: Cancels or deadlines this call.
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
//   - ctx: Cancels or deadlines this call.
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
//   - ctx: Cancels or deadlines this call.
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
//   - ctx: Cancels or deadlines this call.
//   - server: Plex Media Server connection (URL and token).
//   - mediaID: Media id.
//
// Returns:
//   - value: The file path for a media item.
//   - err: Wrapped failure such as "get media path"; "decode media detail".
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
		if file := firstMediaFile(&container.Metadata[index]); file != "" {
			return file, nil
		}
	}

	return "", ErrNoFilePathFound
}

// Ping pings the server to check connectivity.
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//   - server: Plex Media Server connection (URL and token).
//
// Returns:
//   - err: Wrapped failure such as "ping server"; "... ...".
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
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//   - server: Plex Media Server connection (URL and token).
//
// Returns:
//   - serverIdentity: Result of GetServerIdentity.
//   - err: Wrapped failure such as "get server identity"; "decode server
//     identity".
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

// MapPlexType maps Plex type strings to standardized types.
//
// Parameters:
//   - plexType: Typed string argument for MapPlexType.
//
// Returns:
//   - value: The Plex type strings to standardized types.
func MapPlexType(plexType string) string {
	mapped, ok := plexTypeNames[plexType]
	if !ok {
		return "unknown"
	}

	return mapped
}

// jsonHeaders returns PMS JSON request headers as documented by the OpenAPI
// spec.
//
// Parameters:
//   - token: Plex or session access token.
//
// Returns:
//   - values: The PMS JSON request headers as documented by the OpenAPI spec.
func jsonHeaders(token string) map[string]string {
	return map[string]string{
		headerPlexToken: token,
		headerAccept:    acceptJSON,
	}
}

// sectionThumb prefers a section thumb, then the composite image.
//
// Parameters:
//   - section: Typed pms.Section argument for sectionThumb.
//
// Returns:
//   - value: Result value; zero or empty when unavailable.
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
//   - container: Typed pms.Container argument for mediaPage.
//   - start: Typed int argument for mediaPage.
//   - size: Typed int argument for mediaPage.
//
// Returns:
//   - mediaPage: A PMS container onto a page of media items.
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
