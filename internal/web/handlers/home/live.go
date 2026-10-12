// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/web/components/playback"
)

// playbackFeed reports the playback panel of the media item one page shows.
type playbackFeed struct {
	// handler reads the session monitor.
	handler *Handler
	// mediaID is the media item.
	mediaID string
	// scope is whose sessions the stream's caller may see.
	scope clip.Scope
	// sent is the panel last sent, empty before the first.
	sent string
}

// playbackUpdate is a media item's playback panel, sent when it changes.
type playbackUpdate struct {
	// ID is the media item the panel belongs to.
	ID string `json:"id"`
	// HTML is the rendered panel.
	HTML string `json:"html"`
}

// mediaIDPattern matches a Plex media item id.
var mediaIDPattern = regexp.MustCompile(`^\d{1,12}$`)

// LivePlayback starts the feed of a media item's playback panel.
//
// Each check renders the panel from the session monitor's cache, the way the
// playback route does, and reports it when it changed. The monitor reads Plex
// on its own interval, so the panel changes only as often as that.
//
// Parameters:
//   - value: The media item id.
//   - scope: Whose sessions the stream's caller may see.
//
// Returns:
//   - check: The feed's check, nil when the value is not a media item id.
func (handler *Handler) LivePlayback(
	value string,
	scope clip.Scope,
) func(ctx context.Context) []string {
	if !mediaIDPattern.MatchString(value) {
		return nil
	}

	feed := &playbackFeed{handler: handler, mediaID: value, scope: scope, sent: ""}

	return feed.check
}

// check reports the panel when it changed since the last check.
//
// Parameters:
//   - ctx: The stream's context.
//
// Returns:
//   - payloads: The panel, or nothing when it is unchanged or cannot render.
func (feed *playbackFeed) check(ctx context.Context) []string {
	var html strings.Builder

	err := playback.PlaybackPanel(feed.handler.playbackProps(feed.mediaID, feed.scope)).
		Render(ctx, &html)
	if err != nil || html.String() == feed.sent {
		return nil
	}

	feed.sent = html.String()

	// Two strings always encode.
	payload, _ := json.Marshal(playbackUpdate{ID: feed.mediaID, HTML: feed.sent})

	return []string{string(payload)}
}
