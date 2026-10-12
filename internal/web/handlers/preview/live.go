// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"encoding/json"
)

// previewFeed reports the status of the preview one page renders.
type previewFeed struct {
	// handler describes the preview.
	handler *Handler
	// id is the preview the page renders.
	id string
	// sent is the payload last sent, empty before the first.
	sent string
}

// goneUpdate tells a page its preview is no longer registered.
type goneUpdate struct {
	// ID is the preview.
	ID string `json:"id"`
	// Gone is always true.
	Gone bool `json:"gone"`
}

// maxLivePreviewID is the longest preview id a page may follow.
const maxLivePreviewID = 128

// LivePreview starts the feed of the preview a page renders.
//
// Each check reports the preview's status when it changed, in the shape the
// status route answers with. A preview that is no longer registered is
// reported once as gone, because the page's own poll, which would learn that
// from a 404, is held back while the stream delivers.
//
// Parameters:
//   - value: The preview id.
//
// Returns:
//   - check: The feed's check, nil when the value is not a preview id.
func (handler *Handler) LivePreview(value string) func(ctx context.Context) []string {
	if value == "" || len(value) > maxLivePreviewID {
		return nil
	}

	feed := &previewFeed{handler: handler, id: value, sent: ""}

	return feed.check
}

// changed reports a payload that differs from the one last sent.
//
// Parameters:
//   - payload: The preview's status now.
//
// Returns:
//   - payloads: The payload, or nothing when it was already sent.
func (feed *previewFeed) changed(payload string) []string {
	if payload == feed.sent {
		return nil
	}

	feed.sent = payload

	return []string{payload}
}

// check reports the preview's status when it changed since the last check.
//
// Parameters:
//   - ctx: The stream's context.
//
// Returns:
//   - payloads: The status, or nothing when it is unchanged.
func (feed *previewFeed) check(ctx context.Context) []string {
	view, ok := feed.handler.previews.Status(feed.id)
	if !ok {
		// An id and a flag always encode.
		gone, _ := json.Marshal(goneUpdate{ID: feed.id, Gone: true})

		return feed.changed(string(gone))
	}

	payload, err := json.Marshal(feed.handler.statusPayload(ctx, view))
	if err != nil {
		return nil
	}

	return feed.changed(string(payload))
}
