// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	fiberClient "github.com/gofiber/fiber/v3/client"
)

// serverBaseURL returns the scheme://host:port origin for a PMS.
func serverBaseURL(server Server) string {
	scheme := server.Scheme
	if scheme == "" {
		scheme = defaultScheme
	}

	return scheme + "://" + formatHost(server.Address, scheme, server.Port)
}

// getPMS performs an authenticated GET against a Plex Media Server.
func (client *Client) getPMS(
	ctx context.Context,
	server Server,
	path string,
	rawQuery string,
) (*fiberClient.Response, error) {
	reqURL := serverBaseURL(server) + path
	if rawQuery != "" {
		reqURL += "?" + rawQuery
	}

	token := server.Token
	if token == "" {
		token = client.Token
	}

	cfg := newRequestConfig(ctx, jsonHeaders(token), nil)

	resp, err := client.httpClient.Get(reqURL, cfg)
	if err != nil {
		return nil, fmt.Errorf("pms request: %w", err)
	}

	if resp.StatusCode() >= http.StatusBadRequest {
		return nil, fmt.Errorf("%w %d", ErrServerReturnedError, resp.StatusCode())
	}

	return resp, nil
}

// ValidThumbPath reports whether path is a Plex library thumbnail path.
func ValidThumbPath(path string) bool {
	if path == "" || strings.Contains(path, "..") || !strings.HasPrefix(path, "/") {
		return false
	}

	return strings.HasPrefix(path, "/library/") || strings.HasPrefix(path, "/photo/")
}

// GetThumb fetches a thumbnail from the Plex Media Server.
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
	cfg := newRequestConfig(ctx, map[string]string{headerPlexToken: token}, nil)

	resp, err := client.httpClient.Get(reqURL, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("get thumb: %w", err)
	}

	if resp.StatusCode() >= http.StatusBadRequest {
		return nil, "", fmt.Errorf("%w %d", ErrServerReturnedError, resp.StatusCode())
	}

	contentType := resp.Header("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	return resp.Body(), contentType, nil
}

// SearchOnServer searches media via GET /hubs/search.
func (client *Client) SearchOnServer(
	ctx context.Context,
	server Server,
	query string,
) ([]MediaItem, error) {
	resp, err := client.getPMS(ctx, server, "/hubs/search", "query="+url.QueryEscape(query))
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
func (client *Client) GetChildren(
	ctx context.Context,
	server Server,
	mediaID string,
) ([]MediaItem, error) {
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

// GetMediaItem fetches a single media item from a Plex Media Server.
func (client *Client) GetMediaItem(
	ctx context.Context,
	server Server,
	mediaID string,
) (*MediaItem, error) {
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

	item := metadataToItem(container.Metadata[0], "")

	item.ID = mediaID

	return &item, nil
}

// GetSessionsOnServer fetches active sessions from a Plex Media Server.
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
		item := metadataToItem(*meta, "")

		sessions = append(sessions, Session{
			ID:         meta.Session.ID,
			MediaItem:  item,
			Title:      meta.Title,
			Duration:   item.Duration,
			ViewOffset: float64(meta.ViewOffset) / scaleMsToS,
		})
	}

	return sessions, nil
}
