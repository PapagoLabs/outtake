// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// PinResponse represents a PIN response from Plex.
type PinResponse struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
}

// UserResponse represents a user response from Plex.
type UserResponse struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// plexAuthAppBase is the documented Plex Auth App prefix, including "#?".
const plexAuthAppBase = "https://app.plex.tv/auth#?"

// GeneratePIN generates a new PIN for authentication.
func (client *Client) GeneratePIN(ctx context.Context) (*PinResponse, error) {
	cfg := newRequestConfig(ctx, map[string]string{
		"Accept":                   acceptJSON,
		"Content-Type":             "application/x-www-form-urlencoded",
		"X-Plex-Product":           client.Product,
		"X-Plex-Client-Identifier": client.ClientID,
	}, "strong=true")

	resp, err := client.httpClient.Post(
		client.baseURL.ResolveReference(newURL("", "", "/api/v2/pins", "strong=true")).String(),
		cfg,
	)
	if err != nil {
		return nil, fmt.Errorf("generate PIN: %w", err)
	}

	var pin PinResponse

	err = client.decodeResponse(resp, &pin)
	if err != nil {
		return nil, fmt.Errorf("decode PIN response: %w", err)
	}

	return &pin, nil
}

// PollPIN polls for PIN authentication.
func (client *Client) PollPIN(ctx context.Context, pinID int, pinCode string) (string, error) {
	resp, err := client.doRequest(
		ctx,
		"/api/v2/pins/"+strconv.Itoa(pinID),
		"code="+url.QueryEscape(pinCode),
	)
	if err != nil {
		return "", fmt.Errorf("poll PIN: %w", err)
	}

	var tokenResp struct {
		AuthToken string `json:"authToken"`
	}

	err = client.decodeResponse(resp, &tokenResp)
	if err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	if tokenResp.AuthToken == "" {
		return "", ErrPINNotYetClaimed
	}

	return tokenResp.AuthToken, nil
}

// ValidateToken validates the current authentication token.
func (client *Client) ValidateToken(ctx context.Context) (bool, *UserResponse, error) {
	resp, err := client.doRequest(ctx, "/api/v2/user", "")
	if err != nil {
		return false, nil, fmt.Errorf("validate token: %w", err)
	}

	if resp.StatusCode() == http.StatusUnauthorized {
		return false, nil, ErrUnauthorized
	}

	var user UserResponse

	err = client.decodeResponse(resp, &user)
	if err != nil {
		return false, nil, fmt.Errorf("decode user response: %w", err)
	}

	return true, &user, nil
}

// GetAuthURL builds the Plex Auth App URL.
//
// Plex requires parameters in the URL fragment after a literal "#?", not a
// query string. [url.URL.String] encodes that "?" and re-encodes already-escaped
// values, so this concatenates the encoded parameters onto the documented prefix.
func (client *Client) GetAuthURL(pinCode, clientID, forwardURL string) string {
	query := url.Values{}
	query.Set("clientID", clientID)
	query.Set("code", pinCode)
	query.Set("context[device][product]", client.Product)
	query.Set("forwardUrl", forwardURL)

	return plexAuthAppBase + query.Encode()
}
