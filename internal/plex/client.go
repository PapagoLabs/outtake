// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/valyala/fasthttp"

	fiberClient "github.com/gofiber/fiber/v3/client"

	"github.com/PapagoLabs/outtake/internal/logging"
)

// Client represents a Plex API client.
type Client struct {
	httpClient  *fiberClient.Client
	thumbClient *fiberClient.Client
	Token       string
	Product     string
	ClientID    string
	baseURL     *url.URL
}

// ClientConfig represents configuration for the Plex client.
type ClientConfig struct {
	Product  string
	ClientID string
	Token    string
	Timeout  time.Duration
	BaseURL  string
}

const (
	// productName is the default X-Plex-Product value.
	productName = "outtake"

	// defaultTimeout is the HTTP client timeout when none is configured.
	defaultTimeout = 30 * time.Second

	// errorStatusThreshold is the first HTTP status treated as an error.
	errorStatusThreshold = 400

	// defaultScheme is the plex.tv URL scheme.
	defaultScheme = "https"

	// defaultHost is the plex.tv API host.
	defaultHost = "plex.tv"

	// maxResponseBytes caps a Plex API response body.
	maxResponseBytes = 64 << 20

	// maxThumbBytes caps a thumbnail response body.
	maxThumbBytes = 10 << 20
)

var (
	// sharedHTTP is the connection pool every client with the default timeout
	// issues API requests through, so connections and TLS sessions are reused
	// across clients.
	sharedHTTP = sync.OnceValue(func() *fiberClient.Client {
		return newHTTP(defaultTimeout, maxResponseBytes)
	})

	// sharedThumbHTTP is the pool thumbnails are fetched through, capped below
	// the API limit.
	sharedThumbHTTP = sync.OnceValue(func() *fiberClient.Client {
		return newHTTP(defaultTimeout, maxThumbBytes)
	})
)

// NewClient creates a new Plex client with the default Fiber HTTP client.
//
// Parameters:
//   - cfg: Product, client identifier, token, timeout, and optional base URL.
//
// Returns:
//   - client: A client that defaults the timeout and product name.
func NewClient(cfg ClientConfig) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}

	if cfg.Product == "" {
		cfg.Product = productName
	}

	if cfg.Timeout == defaultTimeout {
		return newPlexClient(cfg, sharedHTTP(), sharedThumbHTTP())
	}

	return newPlexClient(
		cfg,
		newHTTP(cfg.Timeout, maxResponseBytes),
		newHTTP(cfg.Timeout, maxThumbBytes),
	)
}

// newHTTP builds a Fiber HTTP client whose responses are capped in size.
//
// Parameters:
//   - timeout: Per-request timeout.
//   - maxBody: Largest response body accepted, in bytes.
//
// Returns:
//   - client: The HTTP client.
func newHTTP(timeout time.Duration, maxBody int) *fiberClient.Client {
	client := fiberClient.NewWithClient(&fasthttp.Client{MaxResponseBodySize: maxBody})
	client.SetTimeout(timeout)

	return client
}

// SetBaseURL sets the base URL for server-specific requests.
//
// Parameters:
//   - rawURL: New request origin.
//
// Returns:
//   - err: Non-nil when rawURL is not a valid URL.
func (client *Client) SetBaseURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse base URL: %w", err)
	}

	client.baseURL = parsed

	return nil
}

// SetToken sets the authentication token.
//
// Parameters:
//   - token: Plex access token.
func (client *Client) SetToken(token string) {
	client.Token = token
}

// defaultBaseURL returns the plex.tv API origin.
//
// Returns:
//   - url: The plex.tv scheme and host.
func defaultBaseURL() *url.URL {
	return &url.URL{Scheme: defaultScheme, Host: defaultHost}
}

// newPlexClient constructs a client and applies an optional custom base URL.
//
// Parameters:
//   - cfg: Product, client identifier, token, timeout, and optional base URL.
//   - httpClient: The HTTP client API requests are issued through.
//   - thumbClient: The HTTP client thumbnails are fetched through.
//
// Returns:
//   - client: A client bound to the HTTP clients.
func newPlexClient(cfg ClientConfig, httpClient, thumbClient *fiberClient.Client) *Client {
	plexClient := &Client{
		httpClient:  httpClient,
		thumbClient: thumbClient,
		Token:       cfg.Token,
		Product:     cfg.Product,
		ClientID:    cfg.ClientID,
		baseURL:     defaultBaseURL(),
	}

	if cfg.BaseURL == "" {
		return plexClient
	}

	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return plexClient
	}

	plexClient.baseURL = parsed

	return plexClient
}

// decodeResponse unmarshals a JSON Plex response.
//
// Parameters:
//   - resp: Plex response to decode.
//   - target: Destination for the decoded body; nil skips decoding.
//
// Returns:
//   - err: Non-nil for a failure status, an empty body, or invalid JSON.
func (*Client) decodeResponse(resp *fiberClient.Response, target any) error {
	if resp.StatusCode() >= errorStatusThreshold {
		return fmt.Errorf("%w %d: %s", ErrPlexError, resp.StatusCode(), string(resp.Body()))
	}

	if target == nil {
		return nil
	}

	body := resp.Body()
	if len(body) == 0 {
		return errEmptyBody
	}

	err := json.Unmarshal(body, target)
	if err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// doRequest sends a GET request to the Plex API.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - path: API path appended to the client base URL.
//   - rawQuery: Encoded query string, or empty for none.
//
// Returns:
//   - resp: The Plex response, with its status unexamined.
//   - err: Non-nil when the request could not be executed.
func (client *Client) doRequest(
	ctx context.Context,
	path, rawQuery string,
) (*fiberClient.Response, error) {
	resp, err := client.requestPlex(ctx, path, rawQuery, acceptJSON)
	if err != nil {
		return nil, fmt.Errorf("plex request: %w", err)
	}

	return resp, nil
}

// requestPlex sends a GET request to the Plex API with the Accept value the
// caller will decode.
//
// Parameters:
//   - ctx: Cancellation and deadline for the request.
//   - path: API path appended to the client base URL.
//   - rawQuery: Encoded query string, or empty for none.
//   - accept: Accept header. plex.tv returns JSON for application/json, which
//     cannot be decoded by the XML parsers.
//
// Returns:
//   - resp: The Plex response, with its status unexamined.
//   - err: Non-nil when the request could not be executed.
func (client *Client) requestPlex(
	ctx context.Context,
	path, rawQuery, accept string,
) (*fiberClient.Response, error) {
	// Send the request against the plex.tv base URL.
	reqURL := client.baseURL.ResolveReference(&url.URL{Path: path, RawQuery: rawQuery})
	headers := map[string]string{
		"Accept":                   accept,
		"X-Plex-Product":           client.Product,
		"X-Plex-Client-Identifier": client.ClientID,
	}

	if client.Token != "" {
		headers["X-Plex-Token"] = client.Token
	}

	cfg := fiberClient.Config{Ctx: ctx, Header: headers}

	logging.Logger.Debug().
		Str("method", "GET").
		Str("url", reqURL.String()).
		Msg("plex request")

	resp, err := client.httpClient.Get(reqURL.String(), cfg)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	return resp, nil
}
