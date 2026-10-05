// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package routes

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestItemURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/media/item/"+"42", ItemURL("42", nil))
	assert.Equal(t, "/media/item/42?start=30", ItemURL("42", url.Values{"start": {"30"}}),
		"the marks are carried so the page can restore the window the user came from")
}

func TestItemURLEscapesID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/media/item/a%2Fb", ItemURL("a/b", nil))
}

func TestLibraryURL(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse(LibraryURL("2"))
	require.NoError(t, err)
	assert.Equal(t, PathMedia, parsed.Path)
	assert.Equal(t, "2", parsed.Query().Get(QueryLibrary))
}

func TestLibraryURLEscapesTheID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/media?library=a%2Fb", LibraryURL("a/b"))
}

func TestBrowseURLCarriesTheDrillDownTrail(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse(
		BrowseURL("2", "10", "Season 3", "9", "Show"),
	)
	require.NoError(t, err)
	assert.Equal(t, PathMedia, parsed.Path)
	assert.Equal(t, "2", parsed.Query().Get(QueryLibrary))
	assert.Equal(t, "10", parsed.Query().Get(QueryParent))
	assert.Equal(t, "Season 3", parsed.Query().Get(QueryTitle))
	assert.Equal(t, "9", parsed.Query().Get(QueryUp))
	assert.Equal(t, "Show", parsed.Query().Get(QueryUpTitle))
}

func TestBrowseURLDropsTheTrailAtALibraryRoot(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse(BrowseURL("2", "9", "Show", "", ""))
	require.NoError(t, err)
	assert.Empty(t, parsed.Query().Get(QueryUp))
	assert.Empty(t, parsed.Query().Get(QueryUpTitle))
}
