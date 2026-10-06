// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package routes

const (
	// ParamID is the route parameter carrying an id.
	ParamID = "id"

	// HeaderHXRequest is the HTMX request marker header.
	HeaderHXRequest = "HX-Request"

	// HeaderHXTarget is the HTMX swap-target header.
	HeaderHXTarget = "HX-Target"

	// HeaderHXCurrentURL is the HTMX header naming the page a request came from.
	HeaderHXCurrentURL = "HX-Current-URL"

	// HeaderHXReswap overrides the swap an HTMX element asked for.
	HeaderHXReswap = "Hx-Reswap"

	// HeaderHXRefresh tells HTMX to reload the page instead of swapping.
	HeaderHXRefresh = "HX-Refresh"

	// HeaderHXRedirect tells HTMX to navigate to another page.
	HeaderHXRedirect = "HX-Redirect"

	// HXTargetNone leaves the targeted element exactly as it was.
	HXTargetNone = "none"
)

// The element ids HTMX swaps media and clip fragments into.
const (
	// TargetMediaBrowse is the poster grid the media page swaps.
	TargetMediaBrowse = "media-browse"

	// TargetMediaMore is the append target for the next poster page.
	TargetMediaMore = "media-more"

	// TargetMediaPrev is the prepend target for the previous poster page.
	TargetMediaPrev = "media-prev"

	// TargetClipList is the clip list the clips page swaps.
	TargetClipList = "clip-list"
)
