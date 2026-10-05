// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/routes"
)

func TestExportFormApplyToQueryLeavesTheZeroFieldsUnwritten(t *testing.T) {
	t.Parallel()

	values := url.Values{}

	ExportForm{}.ApplyToQuery(values)

	assert.NotContains(t, values, routes.QueryExportType)
	assert.NotContains(t, values, routes.QueryQuality)
	assert.NotContains(t, values, routes.QueryAudioIndex)
	assert.NotContains(t, values, routes.QueryWidth)
	assert.NotContains(t, values, routes.QueryFPS)
	assert.Empty(t, values.Get(routes.QueryExportName))
	assert.Equal(t, routes.FormUnchecked, values.Get(routes.QueryCropBlackBars))
	assert.Equal(t, routes.FormUnchecked, values.Get(routes.QueryWebSafeColor))
	assert.Equal(t, routes.FormUnchecked, values.Get(routes.QueryPreserveHDR))
}

func TestExportFormApplyToQueryOmitsAnEmptyExportType(t *testing.T) {
	t.Parallel()

	values := url.Values{routes.QueryExportType: {"gif"}}

	ExportForm{}.ApplyToQuery(values)

	assert.Equal(t, "gif", values.Get(routes.QueryExportType),
		"a form carrying no export type leaves the type already on the query alone")
}

func TestExportFormApplyToQueryClearsACarriedName(t *testing.T) {
	t.Parallel()

	values := url.Values{routes.QueryExportName: {"Stale"}}

	ExportForm{}.ApplyToQuery(values)

	require.Contains(t, values, routes.QueryExportName,
		"the name is always written, so an emptied field reaches the page as an empty one")
	assert.Empty(t, values.Get(routes.QueryExportName))
}

func TestExportFormApplyToQueryCarriesEveryField(t *testing.T) {
	t.Parallel()

	values := url.Values{}

	ExportForm{
		Type:          "gif",
		Name:          "Opening",
		Quality:       "profile-1",
		AudioIndex:    2,
		Width:         640,
		FPS:           12,
		CropBlackBars: true,
		WebSafeColor:  true,
		PreserveHDR:   true,
	}.ApplyToQuery(values)

	assert.Equal(t, "gif", values.Get(routes.QueryExportType))
	assert.Equal(t, "Opening", values.Get(routes.QueryExportName))
	assert.Equal(t, "profile-1", values.Get(routes.QueryQuality))
	assert.Equal(t, "2", values.Get(routes.QueryAudioIndex))
	assert.Equal(t, "640", values.Get(routes.QueryWidth))
	assert.Equal(t, "12", values.Get(routes.QueryFPS))
	assert.Equal(t, routes.FormChecked, values.Get(routes.QueryCropBlackBars))
	assert.Equal(t, routes.FormChecked, values.Get(routes.QueryWebSafeColor))
	assert.Equal(t, routes.FormChecked, values.Get(routes.QueryPreserveHDR))
}
