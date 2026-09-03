// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

// MediaPage is one page of library or container children.
type MediaPage struct {
	Items []MediaItem
	Total int
	Start int
	Size  int
}
