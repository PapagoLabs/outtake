// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package key

import (
	"strings"
)

// Prefix is the Plex metadata path prefix that carries a metadata identifier.
const Prefix = "/library/metadata/"

// ID reduces a Plex metadata path to the metadata identifier it carries.
//
// Parameters:
//   - path: A Plex request path, such as "/library/metadata/42/children".
//
// Returns:
//   - id: The metadata identifier, or empty when path lacks the Prefix.
func ID(path string) string {
	id, ok := strings.CutPrefix(path, Prefix)
	if !ok {
		return ""
	}

	id = strings.TrimSuffix(id, "/")
	if slash := strings.Index(id, "/"); slash >= 0 {
		id = id[:slash]
	}

	return id
}
