// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"net/url"
	"strconv"

	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// MoreClipsURL builds the address of a clip list's next page.
//
// The address keeps the list's filters, sort, and Shared view, and names the
// page's last clip and how many clips came before the next page.
//
// Parameters:
//   - path: The list's route, such as /clips.
//   - query: Normalized toolbar state the page was read with.
//   - page: The page just shown.
//
// Returns:
//   - address: The next page's address, empty when no clip follows.
func MoreClipsURL(path string, query catalog.ClipListQuery, page catalog.ClipPage) string {
	if !page.More {
		return ""
	}

	values := url.Values{}

	for name, value := range map[string]string{
		routes.QueryClipStatus: string(query.Status),
		routes.QueryClipType:   string(query.Type),
		routes.QueryClipName:   query.Query,
		routes.QueryClipSort:   query.Sort,
		routes.QueryClipAfter:  page.Last(),
	} {
		if value != "" {
			values.Set(name, value)
		}
	}

	if query.Shared {
		values.Set(routes.QueryShow, routes.ShowShared)
	}

	values.Set(routes.QueryClipOffset, strconv.Itoa(page.Next))

	return path + "?" + values.Encode()
}
