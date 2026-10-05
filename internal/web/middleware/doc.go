// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package middleware provides HTTP middleware for the outtake API.
package middleware

import (
	"github.com/PapagoLabs/outtake/internal/plex/identity"
)

// SessionKeyToken is the session key used to store the Plex authentication token.
const SessionKeyToken = identity.SessionKeyToken
