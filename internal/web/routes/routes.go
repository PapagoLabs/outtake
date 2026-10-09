// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package routes holds the vocabulary of the web surface: the paths it serves,
// the query parameters it reads, and how a link into it is built.
package routes

import (
	"net/url"
)

const (
	// PathRoot is the dashboard path.
	PathRoot = "/"

	// PathLogin is the login page path.
	PathLogin = "/login"

	// PathClips is the clips page path.
	PathClips = "/clips"

	// PathServers is the server picker path.
	PathServers = "/servers"

	// PathSettingsProfiles is the clip profile settings path.
	PathSettingsProfiles = "/settings/profiles"

	// PathMedia is the media library path.
	PathMedia = "/media"

	// PathItemPrefix is the prefix of a media item page path.
	PathItemPrefix = "/media/item/"

	// PathThumb is the thumbnail cache proxy path.
	PathThumb = "/thumbs"

	// PathPreviewPrefix is the prefix a published preview is served from.
	PathPreviewPrefix = "/previews/"
)

// The query parameters the web surface reads.
const (
	// QueryAudioIndex carries the New export audio track.
	QueryAudioIndex = "audioIndex"

	// QueryBefore is the media offset parameter that pages backwards.
	QueryBefore = "before"

	// QueryCropBlackBars carries the New export crop toggle.
	QueryCropBlackBars = "cropBlackBars"

	// QueryEnd is the New export end mark on a media item page.
	QueryEnd = "end"

	// QueryError is the flash-error parameter on HTML pages.
	QueryError = "error"

	// QueryExportName carries the New export clip name across a redirect.
	QueryExportName = "exportName"

	// QueryExportType carries the New export type across a preview redirect.
	QueryExportType = "exportType"

	// QueryFPS carries the New export GIF frame rate across a redirect.
	QueryFPS = "fps"

	// QueryLibrary is the media library id parameter.
	QueryLibrary = "library"

	// QueryLetter is the media jump index letter parameter.
	QueryLetter = "letter"

	// QueryParent is the media container parent id.
	QueryParent = "parent"

	// QueryPreview is the rendered preview id on a media item page.
	QueryPreview = "preview"

	// QueryPreviewSDR marks a preview of an HDR clip that was tone mapped for
	// an SDR screen.
	QueryPreviewSDR = "previewSdr"

	// QueryScreenHDR is the form field and query parameter that says the
	// browser can show an HDR preview: its screen shows HDR and it decodes
	// HEVC Main 10.
	QueryScreenHDR = "screenHdr"

	// QueryQuality carries the New export profile across a preview redirect.
	QueryQuality = "quality"

	// QuerySession names the Plex session a position is read from.
	QuerySession = "session"

	// QueryStart is the media pagination offset and the New export start mark.
	QueryStart = "start"

	// QueryTitle is the media browse title parameter.
	QueryTitle = "title"

	// QueryThumbPath names the Plex thumb the cache proxy should serve.
	QueryThumbPath = "path"

	// QueryUp is the media breadcrumb parent id.
	QueryUp = "up"

	// QueryUpTitle is the media breadcrumb parent title.
	QueryUpTitle = "upTitle"

	// QueryWidth carries the New export GIF width across a redirect.
	QueryWidth = "width"
)

// The values a checkbox takes on the wire.
const (
	// FormChecked is the value of a checked HTML checkbox.
	FormChecked = "1"

	// FormUnchecked is the value of an explicitly cleared checkbox.
	FormUnchecked = "0"
)

const (
	// queryAssign assigns a value to a query parameter.
	queryAssign = "="

	// queryStart starts the query of a hand-built link.
	queryStart = "?"
)

// ItemURL builds a media item path with a path-escaped id.
//
// Parameters:
//   - mediaID: Plex media item id.
//   - values: Query values to carry onto the item page.
//
// Returns:
//   - location: Media item path for the media id.
func ItemURL(mediaID string, values url.Values) string {
	location := PathItemPrefix + url.PathEscape(mediaID)
	if encoded := values.Encode(); encoded != "" {
		location += queryStart + encoded
	}

	return location
}

// BrowseURL builds a drill-down link for a container item.
//
// Parameters:
//   - libraryID: Library the container belongs to.
//   - itemID: Container rating key.
//   - itemTitle: Container title carried as the trail label.
//   - parentID: Rating key of the container above this one.
//   - parentTitle: Title of the container above this one.
//
// Returns:
//   - location: Media library path carrying the browse trail.
func BrowseURL(libraryID, itemID, itemTitle, parentID, parentTitle string) string {
	values := url.Values{}
	values.Set(QueryLibrary, libraryID)
	values.Set(QueryParent, itemID)
	values.Set(QueryTitle, itemTitle)

	if parentID != "" {
		values.Set(QueryUp, parentID)
		values.Set(QueryUpTitle, parentTitle)
	}

	return PathMedia + queryStart + values.Encode()
}

// LibraryURL builds the path that opens a library root.
//
// Parameters:
//   - libraryID: Library to open.
//
// Returns:
//   - location: Media library path scoped to the library.
func LibraryURL(libraryID string) string {
	return PathMedia + queryStart + QueryLibrary + queryAssign + url.QueryEscape(libraryID)
}

// CheckedValue encodes a checkbox state as the value a query carries.
//
// Parameters:
//   - checked: Whether the box is checked.
//
// Returns:
//   - value: FormChecked when checked, otherwise FormUnchecked.
//
//nolint:revive // The checked state is the value being encoded, not a control switch.
func CheckedValue(checked bool) string {
	if checked {
		return FormChecked
	}

	return FormUnchecked
}

// IsFormChecked reads a wire value back as a checkbox state.
//
// Parameters:
//   - raw: Value read from a form field or query parameter.
//
// Returns:
//   - on: True when the value is the checked one.
func IsFormChecked(raw string) bool {
	return raw == FormChecked
}
