// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// NewBoundClient constructs a Plex client for the given token.
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
func ClipMatchesStatus(itemStatus, want string) bool {
	switch want {
	case view.ClipStatusPending:
		return itemStatus == view.ClipStatusPending || itemStatus == view.ClipStatusProcessing
	default:
		return itemStatus == want
	}
}
