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
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//
// Returns:
//   - pinResponse: Result of GeneratePIN.
//   - err: Wrapped failure such as "generate PIN"; "decode PIN response".
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
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//   - pinID: Typed int argument for PollPIN.
//   - pinCode: Typed string argument for PollPIN.
//
// Returns:
//   - value: Result value; zero or empty when unavailable.
//   - err: Wrapped failure such as "poll PIN"; "decode token response".
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
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//
// Returns:
//   - ok: True when the condition holds.
//   - userResponse: Result of ValidateToken.
//   - err: Wrapped failure such as "validate token"; "decode user response".
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
// Parameters:
//   - pinCode: Typed string argument for GetAuthURL.
//   - clientID: Plex X-Plex-Client-Identifier.
//   - forwardURL: Forward url.
//
// Returns:
//   - url: The Plex Auth App URL.
func (client *Client) GetAuthURL(pinCode, clientID, forwardURL string) string {
	query := url.Values{}
	query.Set("clientID", clientID)
	query.Set("code", pinCode)
	query.Set("context[device][product]", client.Product)
	query.Set("forwardUrl", forwardURL)

	return plexAuthAppBase + query.Encode()
}
