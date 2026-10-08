// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"net/url"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/exportform"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// previewRedirect builds the media item location a preview request returns to.
//
// Parameters:
//   - req: Parsed request carrying the clip bounds and the clip's own choices.
//   - previewID: Id the preview is rendered under.
//
// Returns:
//   - location: A path-only redirect target.
func previewRedirect(req api.ClipRequest, previewID string) string {
	return previewRedirectURL(
		req.MediaID,
		previewID,
		req.StartTime,
		req.StartTime+req.Duration,
		exportform.FromRequest(req),
	)
}

// shownInSDR marks a preview redirect as the tone mapped preview of an HDR
// clip, so the page can say so.
//
// Parameters:
//   - location: A redirect from previewRedirect, which always carries a query.
//
// Returns:
//   - marked: The redirect with the SDR marker added.
func shownInSDR(location string) string {
	return location + "&" + routes.QueryPreviewSDR + "=" + routes.FormChecked
}

// previewRedirectURL builds the media item location that carries a rendered
// preview and the submitted export form back to the form.
//
// Parameters:
//   - mediaID: Plex rating key of the source item.
//   - previewID: Storage id of the rendered preview.
//   - start: Start mark in seconds.
//   - end: End mark in seconds.
//   - form: Export form to carry across the redirect.
//
// Returns:
//   - location: A path-only redirect target.
func previewRedirectURL(
	mediaID, previewID string,
	start, end float64,
	form view.ExportForm,
) string {
	values := url.Values{}
	values.Set(routes.QueryPreview, previewID)
	values.Set(routes.QueryStart, timecode.FormatSeconds(start))
	values.Set(routes.QueryEnd, timecode.FormatSeconds(end))
	form.ApplyToQuery(values)

	return routes.ItemURL(mediaID, values)
}
