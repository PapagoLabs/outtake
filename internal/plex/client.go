// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	fiberClient "github.com/gofiber/fiber/v3/client"

	"github.com/PapagoLabs/outtake/internal/logging"
)

// HTTPClient defines the interface for HTTP operations.
type HTTPClient interface {
	Get(requestURL string, cfg ...fiberClient.Config) (*fiberClient.Response, error)
	Post(requestURL string, cfg ...fiberClient.Config) (*fiberClient.Response, error)
}

// FiberClient wraps the Fiber v3 client to implement HTTPClient.
type FiberClient struct {
	client *fiberClient.Client
}

// Client represents a Plex API client.
type Client struct {
	httpClient HTTPClient
	Token      string
	Product    string
	ClientID   string
	baseURL    *url.URL
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
)

// Ensure FiberClient implements HTTPClient.
var _ HTTPClient = (*FiberClient)(nil)

// Get sends a GET request.
//
// Parameters:
//   - requestURL: Request url.
//   - cfg: Application configuration.
//
// Returns:
//   - resp: The resp.
//   - err: The error, if any.
func (client *FiberClient) Get(
	requestURL string,
	cfg ...fiberClient.Config,
) (*fiberClient.Response, error) {
	// Forward the GET to the Fiber client.
	resp, err := client.client.Get(requestURL, cfg...)
	if err != nil {
		return nil, fmt.Errorf("fiber get: %w", err)
	}

	return resp, nil
}

// Post sends a POST request.
//
// Parameters:
//   - requestURL: Request url.
//   - cfg: Application configuration.
//
// Returns:
//   - resp: The resp.
//   - err: The error, if any.
func (client *FiberClient) Post(
	requestURL string,
	cfg ...fiberClient.Config,
) (*fiberClient.Response, error) {
	// Forward the POST to the Fiber client.
	resp, err := client.client.Post(requestURL, cfg...)
	if err != nil {
		return nil, fmt.Errorf("fiber post: %w", err)
	}

	return resp, nil
}

// NewClient creates a new Plex client with the default Fiber HTTP client.
//
// Parameters:
//   - cfg: Application configuration.
//
// Returns:
//   - client: A new Plex client with the default Fiber HTTP client.
func NewClient(cfg ClientConfig) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}

	if cfg.Product == "" {
		cfg.Product = productName
	}

	fiberHTTP := fiberClient.New()
	fiberHTTP.SetTimeout(cfg.Timeout)

	return newPlexClient(cfg, &FiberClient{client: fiberHTTP})
}

// NewClientWithHTTPClient creates a new Plex client with a custom HTTP client.
//
// Parameters:
//   - cfg: Application configuration.
//   - httpClient: Http client.
//
// Returns:
//   - client: A new Plex client with a custom HTTP client.
func NewClientWithHTTPClient(cfg ClientConfig, httpClient HTTPClient) *Client {
	if cfg.Product == "" {
		cfg.Product = productName
	}

	return newPlexClient(cfg, httpClient)
}

// SetBaseURL sets the base URL for server-specific requests.
//
// Parameters:
//   - rawURL: Raw url.
//
// Returns:
//   - err: The error, if any.
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
//   - token: Token.
func (client *Client) SetToken(token string) {
	client.Token = token
}

// newPlexClient constructs a client and applies an optional custom base URL.
//
// Parameters:
//   - cfg: Application configuration.
//   - httpClient: Http client.
//
// Returns:
//   - client: A client and applies an optional custom base URL.
func newPlexClient(cfg ClientConfig, httpClient HTTPClient) *Client {
	plexClient := &Client{
		httpClient: httpClient,
		Token:      cfg.Token,
		Product:    cfg.Product,
		ClientID:   cfg.ClientID,
		baseURL:    defaultBaseURL(),
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
//   - resp: Resp.
//   - target: Target.
//
// Returns:
//   - err: The error, if any.
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
//   - ctx: Cancellation context.
//   - path: Filesystem path.
//   - rawQuery: Raw query.
//
// Returns:
//   - resp: The resp.
//   - err: The error, if any.
func (client *Client) doRequest(
	ctx context.Context,
	path, rawQuery string,
) (*fiberClient.Response, error) {
	// Send the request against the plex.tv base URL.
	reqURL := client.baseURL.ResolveReference(newURL("", "", path, rawQuery))
	headers := map[string]string{
		"Accept":                   acceptJSON,
		"X-Plex-Product":           client.Product,
		"X-Plex-Client-Identifier": client.ClientID,
	}

	if client.Token != "" {
		headers["X-Plex-Token"] = client.Token
	}

	cfg := newRequestConfig(ctx, headers, nil)

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
