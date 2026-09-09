// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

// AuthStatusResponse represents the authentication status response.
type AuthStatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	UserID        int    `json:"userId,omitempty"`
	Username      string `json:"username,omitempty"`
}
