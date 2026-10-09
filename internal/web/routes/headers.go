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

// The Cross-Origin-Opener-Policy values pages are served with.
const (
	// HeaderOpenerPolicy is the Cross-Origin-Opener-Policy header.
	HeaderOpenerPolicy = "Cross-Origin-Opener-Policy"

	// OpenerPolicyAllowPopups isolates every page from cross-origin openers,
	// while a page keeps its hold on the Plex sign-in popup it opens.
	OpenerPolicyAllowPopups = "same-origin-allow-popups"

	// OpenerPolicyUnsafeNone lets the page Plex returns the sign-in popup to
	// reach the page that opened it. Any other value would cut the popup off
	// from its opener on the way back from Plex, which sends none.
	OpenerPolicyUnsafeNone = "unsafe-none"
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
