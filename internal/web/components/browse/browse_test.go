// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package browse

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestMediaResultsEmptyLibraryHidesChooser(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaResults(view.MediaProps{
		Items: nil,
		Libraries: []view.LibraryItem{
			{ID: "1", Title: "Movies", Type: "movie"},
		},
		Crumbs: []view.Crumb{
			{Title: "Libraries", URL: "/media"},
			{Title: "Movies", URL: "/media?library=1"},
		},
		Query:     "",
		LibraryID: "1",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "No media found")
	assert.NotContains(t, body, ">Browse<")
}

func TestMediaResultsEmptyFolderHidesChooser(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaResults(view.MediaProps{
		Items: nil,
		Libraries: []view.LibraryItem{
			{ID: "1", Title: "Movies", Type: "movie"},
		},
		Crumbs: []view.Crumb{
			{Title: "Libraries", URL: "/media"},
			{Title: "Movies", URL: "/media?library=1"},
			{Title: "Show", URL: "/media?library=1&parent=2"},
		},
		Query:     "",
		LibraryID: "1",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "No media found")
	assert.Contains(t, body, "Show")
	assert.NotContains(t, body, ">Browse<")
}
