// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
)

// TestMoreClipsURLCarriesTheListOn covers the next page's address.
//
// It keeps every filter, the sort, and the Shared view, names the last clip
// shown and the offset, and is empty when no clip follows.
func TestMoreClipsURLCarriesTheListOn(t *testing.T) {
	t.Parallel()

	query := catalog.Normalize(catalog.ClipListQuery{
		Status: clip.StatusCompleted,
		Type:   clip.TypeGIF,
		Query:  "intro & outro",
		Sort:   catalog.SortNameAsc,
		Shared: true,
	})
	page := catalog.ClipPage{Jobs: []*clip.Job{{ID: "last"}}, Next: 24, More: true}

	address, err := url.Parse(MoreClipsURL("/clips", query, page))
	require.NoError(t, err)

	assert.Equal(t, "/clips", address.Path)
	assert.Equal(t, url.Values{
		"status": {"completed"},
		"type":   {"gif"},
		"q":      {"intro & outro"},
		"sort":   {catalog.SortNameAsc},
		"show":   {"shared"},
		"after":  {"last"},
		"offset": {"24"},
	}, address.Query())

	page.More = false

	assert.Empty(t, MoreClipsURL("/clips", query, page))
}
