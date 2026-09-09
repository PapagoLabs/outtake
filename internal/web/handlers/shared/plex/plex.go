// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"github.com/PapagoLabs/outtake/internal/plex"
	viewclip "github.com/PapagoLabs/outtake/internal/web/view/clip"
)

// NewBoundClient constructs a Plex client for the given token.
//
// Parameters:
//   - product: Product.
//   - clientID: Client id.
//   - token: Token.
//
// Returns:
//   - client: A Plex client for the given token.
func NewBoundClient(product, clientID, token string) *plex.Client {
	return plex.NewClient(plex.ClientConfig{
		Product:  product,
		ClientID: clientID,
		Token:    token,
		Timeout:  0,
		BaseURL:  "",
	})
}

// ClipMatchesStatus reports whether a clip belongs to a status filter.
//
// Parameters:
//   - itemStatus: Item status.
//   - want: Want.
//
// Returns:
//   - ok: True when a clip belongs to a status filter.
func ClipMatchesStatus(itemStatus, want string) bool {
	switch want {
	case viewclip.ClipStatusPending:
		return itemStatus == viewclip.ClipStatusPending || itemStatus == viewclip.ClipStatusProcessing
	default:
		return itemStatus == want
	}
}
