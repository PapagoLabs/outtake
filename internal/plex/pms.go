// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	fiberClient "github.com/gofiber/fiber/v3/client"
)

// thumbPathPatterns are the artwork paths a Plex server hands out: art on a
// metadata item, and the composites a section or collection is drawn with. A
// trailing number is the artwork's version.
var thumbPathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^/library/metadata/\d+/(thumb|art|banner|clearLogo)(/\d+)?$`),
	regexp.MustCompile(`^/library/(sections|collections)/\d+/(composite|thumb|art)(/\d+)?$`),
}

// serverBaseURL returns the scheme://host:port origin for a PMS.
//
// Parameters:
//   - server: Server whose address, port, and scheme form the origin.
//
// Returns:
//   - baseURL: The origin, defaulting the scheme when it is unset.
func serverBaseURL(server Server) string {
	scheme := server.Scheme
	if scheme == "" {
		scheme = defaultScheme
	}

	return scheme + "://" + formatHost(server.Address, scheme, server.Port)
}

// getPMS performs an authenticated GET against a Plex Media Server.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to query.
//   - path: Library API path appended to the server origin.
//   - rawQuery: Encoded query string, or empty for none.
//
// Returns:
//   - resp: The PMS response.
//   - err: ErrServerReturnedError for a failure status, also wrapping
//     ErrUnauthorized when the server rejected the token, or a request error.
func (client *Client) getPMS(
	ctx context.Context,
	server Server,
	path string,
	rawQuery string,
) (*fiberClient.Response, error) {
	// Send the request against the PMS base URL.
	reqURL := serverBaseURL(server) + path
	if rawQuery != "" {
		reqURL += "?" + rawQuery
	}

	token := server.Token
	if token == "" {
		token = client.Token
	}

	cfg := fiberClient.Config{Ctx: ctx, Header: jsonHeaders(token)}

	resp, err := client.httpClient.Get(reqURL, cfg)
	if err != nil {
		return nil, fmt.Errorf("pms request: %w", err)
	}

	switch status := resp.StatusCode(); {
	case status == http.StatusUnauthorized:
		return nil, fmt.Errorf("%w %d: %w", ErrServerReturnedError, status, ErrUnauthorized)
	case status >= http.StatusBadRequest:
		return nil, fmt.Errorf("%w %d", ErrServerReturnedError, status)
	default:
		return resp, nil
	}
}

// ValidThumbPath reports whether path is a Plex artwork path. Anything else,
// including a query string or an escaped character, is refused, so the
// thumbnail proxy cannot reach any other Plex endpoint.
//
// Parameters:
//   - path: Candidate thumbnail path.
//
// Returns:
//   - ok: True when path names artwork.
func ValidThumbPath(path string) bool {
	for _, pattern := range thumbPathPatterns {
		if pattern.MatchString(path) {
			return true
		}
	}

	return false
}

// GetThumb fetches a thumbnail from the Plex Media Server.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS holding the asset.
//   - path: Thumbnail path, as accepted by ValidThumbPath.
//
// Returns:
//   - body: The thumbnail bytes.
//   - contentType: The image content type.
//   - err: ErrInvalidThumbPath for an unusable path, ErrNotImage for a response
//     that is not an image, or a request error. A response larger than 10 MiB
//     fails as a request error.
func (client *Client) GetThumb(
	ctx context.Context,
	server Server,
	path string,
) ([]byte, string, error) {
	if !ValidThumbPath(path) {
		return nil, "", ErrInvalidThumbPath
	}

	token := server.Token
	if token == "" {
		token = client.Token
	}

	reqURL := serverBaseURL(server) + path
	cfg := fiberClient.Config{
		Ctx:    ctx,
		Header: map[string]string{headerPlexToken: token},
	}

	resp, err := client.thumbClient.Get(reqURL, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("get thumb: %w", err)
	}

	if resp.StatusCode() >= http.StatusBadRequest {
		return nil, "", fmt.Errorf("%w %d", ErrServerReturnedError, resp.StatusCode())
	}

	body := resp.Body()

	// A missing header reads as fasthttp's text/plain default, so a type that is
	// not an image is checked against the bytes before the response is refused.
	contentType := resp.Header("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		contentType = http.DetectContentType(body)
	}

	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("%w: %s", ErrNotImage, contentType)
	}

	return body, contentType, nil
}

// SearchOnServer searches media via GET /hubs/search.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to query.
//   - query: Free-text search query.
//   - sectionID: Library section key, or empty to search the whole server.
//
// Returns:
//   - items: Metadata from every hub the server returned.
//   - err: Non-nil when the PMS request or decode fails.
func (client *Client) SearchOnServer(
	ctx context.Context,
	server Server,
	query, sectionID string,
) ([]MediaItem, error) {
	rawQuery := "query=" + url.QueryEscape(query)
	if sectionID != "" {
		rawQuery += "&sectionId=" + url.QueryEscape(sectionID)
	}

	resp, err := client.getPMS(ctx, server, "/hubs/search", rawQuery)
	if err != nil {
		return nil, fmt.Errorf("search hubs: %w", err)
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return nil, fmt.Errorf("decode search: %w", decodeErr)
	}

	items := make([]MediaItem, 0)
	for _, hub := range container.Hub {
		items = append(items, metadataItems(hub.Metadata, hub.Title)...)
	}

	return items, nil
}

// GetChildren lists one level of children for a show, season, artist, or album.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to query.
//   - mediaID: Rating key of the container.
//
// Returns:
//   - items: The container's children.
//   - err: Non-nil when both requests and the decode fail.
func (client *Client) GetChildren(
	ctx context.Context,
	server Server,
	mediaID string,
) ([]MediaItem, error) {
	// List child metadata for a container.
	path := "/library/metadata/" + url.PathEscape(mediaID) + "/children"

	resp, err := client.getPMS(ctx, server, path, "")
	if err != nil {
		resp, err = client.getPMS(
			ctx,
			server,
			"/library/metadata/"+url.PathEscape(mediaID)+"/allLeaves",
			"",
		)
		if err != nil {
			return nil, fmt.Errorf("get children: %w", err)
		}
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return nil, fmt.Errorf("decode children: %w", decodeErr)
	}

	return metadataItems(container.Metadata, ""), nil
}

// GetChildrenPage fetches one page of children for a container.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to query.
//   - mediaID: Rating key of the container.
//   - start: Container offset.
//   - size: Page size, or 0 for the PMS default.
//
// Returns:
//   - page: Children and total size for the requested window.
//   - err: Non-nil when both requests and the decode fail.
func (client *Client) GetChildrenPage(
	ctx context.Context,
	server Server,
	mediaID string,
	start, size int,
) (MediaPage, error) {
	path := "/library/metadata/" + url.PathEscape(mediaID) + "/children"

	resp, err := client.getPMS(ctx, server, path, containerQuery(start, size))
	if err != nil {
		resp, err = client.getPMS(
			ctx,
			server,
			"/library/metadata/"+url.PathEscape(mediaID)+"/allLeaves",
			containerQuery(start, size),
		)
		if err != nil {
			return MediaPage{}, fmt.Errorf("get children: %w", err)
		}
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return MediaPage{}, fmt.Errorf("decode children: %w", decodeErr)
	}

	return mediaPage(container, start, size), nil
}

// GetMediaItem fetches a single media item from a Plex Media Server.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to query.
//   - mediaID: Rating key of the item.
//
// Returns:
//   - item: The mapped media item, keyed by mediaID, with the path of its
//     first media part as the server sees it.
//   - err: ErrNoFilePathFound when the server returned no metadata, or a request
//     error.
func (client *Client) GetMediaItem(
	ctx context.Context,
	server Server,
	mediaID string,
) (*MediaItem, error) {
	// Fetch one metadata item from the PMS.
	resp, err := client.getPMS(ctx, server, "/library/metadata/"+url.PathEscape(mediaID), "")
	if err != nil {
		return nil, fmt.Errorf("get media item: %w", err)
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return nil, fmt.Errorf("decode media item: %w", decodeErr)
	}

	if len(container.Metadata) == 0 {
		return nil, ErrNoFilePathFound
	}

	item := metadataToItem(&container.Metadata[0], "")

	item.ID = mediaID
	item.FilePath = firstMediaFile(&container.Metadata[0])

	return &item, nil
}

// GetSessionsOnServer fetches active sessions from a Plex Media Server.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - server: PMS to query.
//
// Returns:
//   - sessions: The server's playback sessions.
//   - err: Non-nil when the PMS request or decode fails.
func (client *Client) GetSessionsOnServer(ctx context.Context, server Server) ([]Session, error) {
	resp, err := client.getPMS(ctx, server, "/status/sessions", "")
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}

	container, decodeErr := decodePMS(resp.Body())
	if decodeErr != nil {
		return nil, fmt.Errorf("decode sessions: %w", decodeErr)
	}

	sessions := make([]Session, 0, len(container.Metadata))
	for index := range container.Metadata {
		meta := &container.Metadata[index]
		item := metadataToItem(meta, "")

		sessions = append(sessions, Session{
			ID:         meta.Session.ID,
			MediaItem:  item,
			Title:      item.DisplayTitle(),
			Duration:   item.Duration,
			ViewOffset: float64(meta.ViewOffset) / scaleMsToS,
		})
	}

	return sessions, nil
}
