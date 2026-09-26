// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"regexp"
)

// previewIDPattern is the safe subset of characters a storage id may contain.
//
// It is deliberately narrower than the ids in use today. Preview ids are
// UUIDs, and cached previews are planned to key on a content hash, so an
// allowlist of unreserved URL characters covers both without having to be
// revisited when the id scheme changes. Anything outside it is rejected before
// the value is joined into a filesystem path.
var previewIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// validPreviewID reports whether an id is safe to use as a storage filename.
//
// The route parameter reaches this unescaped, so a value like `../secret` never
// arrives intact today, and the `.mp4` suffix would turn a bare `..` into an
// ordinary filename. Both are accidents of the current wiring rather than
// guarantees, and the `.mp4` suffix disappears the moment previews are keyed
// on a bare content hash. Rejecting the unsafe characters explicitly keeps the
// path safe under any of those changes.
//
// Parameters:
//   - id: Route parameter naming a stored object.
//
// Returns:
//   - safe: True when the id contains only unreserved characters.
func validPreviewID(id string) bool {
	return previewIDPattern.MatchString(id)
}
