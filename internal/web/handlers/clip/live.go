// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	clipcard "github.com/PapagoLabs/outtake/internal/web/components/clip"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// storedFiles is which of a clip's files the storage holds.
type storedFiles struct {
	// output reports that the clip's own file is stored.
	output bool
	// sdr reports that the clip's SDR version is stored.
	sdr bool
}

// liveUpdate is one clip's status region, sent when it changes.
type liveUpdate struct {
	// ID is the clip the region belongs to.
	ID string `json:"id"`
	// HTML is the rendered status region.
	HTML string `json:"html"`
}

// fileCheck is a clip's stored files as last read, and the state they were
// read in.
type fileCheck struct {
	// state is the clip's status and stage when the files were read.
	state string
	// files is what the storage held then.
	files storedFiles
}

// clipFeed reports the status regions of the clips one page shows.
type clipFeed struct {
	// lookup finds a clip, nil when it is gone.
	lookup func(ctx context.Context, id string) *clipdom.Job
	// render writes a clip's status region.
	render func(ctx context.Context, job *clipdom.Job) (string, error)
	// sent is the region last sent for each clip.
	sent map[string]string
	// ids are the clips the page shows.
	ids []string
}

// statusRenderer renders status regions for one stream.
//
// It reads the profiles and the usernames once and a clip's files only when
// its status or stage changes.
type statusRenderer struct {
	// handler builds the card models.
	handler *Handler
	// files is each clip's stored files as last read.
	files map[string]fileCheck
	// audience is who the cards are shown to, its names read on first use.
	audience view.Audience
	// profiles are the profiles every card offers, read on first use.
	profiles []profile.ProfileOption
	// loaded reports that profiles and names were read.
	loaded bool
}

const (
	// liveIDSeparator separates the clip ids a page follows.
	liveIDSeparator = ","
	// maxLiveClips is the most clips one stream follows.
	maxLiveClips = 50
)

// LiveClips starts the feed of the clip cards a page shows while they render.
//
// The value names the clips, separated by commas. Each check reports the
// status region of every clip whose region changed, rendered the way
// /clips/:id/row renders it.
//
// Parameters:
//   - value: The clip ids, separated by commas.
//   - viewpoint: What the stream's caller may reach. A clip the caller may
//     not see is never reported.
//
// Returns:
//   - check: The feed's check, nil when the value names no clip.
func (handler *Handler) LiveClips(
	value string,
	viewpoint middleware.Viewpoint,
) func(ctx context.Context) []string {
	ids := liveIDs(value)
	if len(ids) == 0 {
		return nil
	}

	renderer := &statusRenderer{
		handler:  handler,
		files:    make(map[string]fileCheck, len(ids)),
		audience: view.Audience{Names: nil, Scope: viewpoint.Scope},
		profiles: nil,
		loaded:   false,
	}

	feed := &clipFeed{
		lookup: func(ctx context.Context, id string) *clipdom.Job {
			return handler.viewJob(ctx, viewpoint, id)
		},
		render: renderer.render,
		sent:   make(map[string]string, len(ids)),
		ids:    ids,
	}

	return feed.check
}

// filesOf returns which of a clip's files are stored.
//
// The storage is asked again only when the clip's status or stage changed,
// because files are published only as a render moves on. On S3 each ask is a
// request.
//
// Parameters:
//   - ctx: The stream's context.
//   - job: The clip.
//
// Returns:
//   - files: Which of the clip's files are stored.
func (renderer *statusRenderer) filesOf(ctx context.Context, job *clipdom.Job) storedFiles {
	state := string(job.Status) + "/" + string(job.Stage)

	if check, ok := renderer.files[job.ID]; ok && check.state == state {
		return check.files
	}

	files := renderer.handler.storedFiles(ctx, job)

	renderer.files[job.ID] = fileCheck{state: state, files: files}

	return files
}

// render writes a clip's status region.
//
// Parameters:
//   - ctx: The stream's context.
//   - job: The clip.
//
// Returns:
//   - html: The rendered region.
//   - err: Non-nil when the region cannot be rendered.
func (renderer *statusRenderer) render(ctx context.Context, job *clipdom.Job) (string, error) {
	if !renderer.loaded {
		renderer.profiles = profile.SelectableProfiles(ctx, renderer.handler.db)
		renderer.audience = renderer.handler.audienceFor(ctx, renderer.audience.Scope)
		renderer.loaded = true
	}

	item := renderer.handler.statusItem(
		job,
		renderer.profiles,
		renderer.filesOf(ctx, job),
		renderer.audience,
	)

	var html strings.Builder

	//nolint:contextcheck // Render takes ctx, which templ's generated closures carry on.
	err := clipcard.ClipStatus(item).Render(ctx, &html)
	if err != nil {
		return "", fmt.Errorf("render clip status: %w", err)
	}

	return html.String(), nil
}

// check reports the regions that changed since the last check.
//
// A clip that is gone, or whose region cannot be rendered, is skipped, and
// the next check tries it again.
//
// Parameters:
//   - ctx: The stream's context.
//
// Returns:
//   - payloads: One liveUpdate for each clip whose region changed.
func (feed *clipFeed) check(ctx context.Context) []string {
	var payloads []string

	for _, id := range feed.ids {
		job := feed.lookup(ctx, id)
		if job == nil {
			continue
		}

		html, err := feed.render(ctx, job)
		if err != nil || html == feed.sent[id] {
			continue
		}

		// Two strings always encode.
		payload, _ := json.Marshal(liveUpdate{ID: id, HTML: html})

		payloads = append(payloads, string(payload))
		feed.sent[id] = html
	}

	return payloads
}

// liveIDs reads the clips a page follows, without repeats, up to
// maxLiveClips.
//
// Parameters:
//   - raw: The clip ids, separated by commas.
//
// Returns:
//   - ids: The clips to follow, in the order given.
func liveIDs(raw string) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0)

	for id := range strings.SplitSeq(raw, liveIDSeparator) {
		id = strings.TrimSpace(id)

		if _, repeat := seen[id]; id == "" || repeat {
			continue
		}

		seen[id] = struct{}{}
		ids = append(ids, id)

		if len(ids) == maxLiveClips {
			break
		}
	}

	return ids
}
