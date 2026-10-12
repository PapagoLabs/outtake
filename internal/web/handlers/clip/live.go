// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	clipcard "github.com/PapagoLabs/outtake/internal/web/components/clip"
	"github.com/PapagoLabs/outtake/internal/web/respond"
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

// clipStream sends the status regions of the clips one page shows, each time
// one changes.
type clipStream struct {
	// lookup finds a clip, nil when it is gone.
	lookup func(ctx context.Context, id string) *clipdom.Job
	// render writes a clip's status region.
	render func(ctx context.Context, job *clipdom.Job) (string, error)
	// sent is the region last sent for each clip.
	sent map[string]string
	// ids are the clips the page shows.
	ids []string
}

// statusRenderer renders status regions for one stream, reading the profiles
// once and a clip's files only when its status or stage changes.
type statusRenderer struct {
	// handler builds the card models.
	handler *Handler
	// files is each clip's stored files as last read.
	files map[string]fileCheck
	// profiles are the profiles every card offers, read on first use.
	profiles []profile.ProfileOption
	// loaded reports that profiles were read.
	loaded bool
}

const (
	// liveIDsQuery names the clips a stream follows, separated by commas.
	liveIDsQuery = "ids"
	// liveIDSeparator separates the ids in liveIDsQuery.
	liveIDSeparator = ","
	// maxLiveClips is the most clips one stream follows.
	maxLiveClips = 50
	// liveTick is how often a stream checks its clips. The card's fallback
	// poll and its progress bar's glide use the same interval.
	liveTick = time.Second
	// liveLifetime is how long one stream runs. The browser then reconnects
	// through the auth guard, so a revoked session or a logout ends it.
	liveLifetime = time.Minute
	// liveRetry tells the browser to reconnect a second after a stream ends.
	liveRetry = "retry: 1000\n\n"
	// liveEventClip names an event carrying a liveUpdate.
	liveEventClip = "clip"
	// liveEventPing names the event every check ends with, which tells the
	// page the stream still delivers.
	liveEventPing = "ping"
	// livePingData is the body of a ping, which must not be empty for the
	// browser to deliver it.
	livePingData = "{}"
	// contentTypeEventStream is the media type of a server-sent event stream.
	contentTypeEventStream = "text/event-stream"
	// headerAccelBuffering asks a reverse proxy not to buffer the stream.
	headerAccelBuffering = "X-Accel-Buffering"
	// cacheNoStore keeps the stream out of every cache.
	cacheNoStore = "no-store"
)

// ClipLive streams the status regions of the clips a page shows while they
// render.
//
// The page names its rendering clips in the ids query. Each region is sent
// when it changes, and every check ends with a ping. A request naming no clip
// answers 204, which tells the browser not to reconnect.
//
// Parameters:
//   - ctx: Request naming the clips to follow.
//
// Returns:
//   - err: Non-nil when the response cannot be started.
func (handler *Handler) ClipLive(ctx fiber.Ctx) error {
	ids := liveIDs(ctx.Query(liveIDsQuery))
	if len(ids) == 0 {
		return respond.SendStatusCode(ctx, fiber.StatusNoContent)
	}

	ctx.Set(fiber.HeaderContentType, contentTypeEventStream)
	ctx.Set(fiber.HeaderCacheControl, cacheNoStore)
	ctx.Set(headerAccelBuffering, "no")

	stream := handler.newClipStream(ids)

	err := ctx.SendStreamWriter(func(writer *bufio.Writer) {
		streamCtx, cancel := handler.streamContext()
		defer cancel()

		stream.run(streamCtx, writer)
	})
	if err != nil {
		return fmt.Errorf("start clip stream: %w", err)
	}

	return nil
}

// newClipStream builds the stream for the clips a page shows.
//
// Parameters:
//   - ids: The clips to follow.
//
// Returns:
//   - stream: A stream that renders through this handler.
func (handler *Handler) newClipStream(ids []string) *clipStream {
	renderer := &statusRenderer{
		handler:  handler,
		files:    make(map[string]fileCheck, len(ids)),
		profiles: nil,
		loaded:   false,
	}

	return &clipStream{
		lookup: handler.lookupJob,
		render: renderer.render,
		sent:   make(map[string]string, len(ids)),
		ids:    ids,
	}
}

// streamContext returns the context a stream runs in, canceled when the
// server shuts down.
//
// The request's own context ends when the handler returns, before the stream
// starts.
//
// Returns:
//   - ctx: The stream's context.
//   - cancel: Releases the context once the stream ends.
func (handler *Handler) streamContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-handler.streamsDone:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
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
		renderer.loaded = true
	}

	item := renderer.handler.statusItem(job, renderer.profiles, renderer.filesOf(ctx, job))

	var html strings.Builder

	err := clipcard.ClipStatus(item).Render(ctx, &html)
	if err != nil {
		return "", fmt.Errorf("render clip status: %w", err)
	}

	return html.String(), nil
}

// run sends the clips' regions until the stream ends.
//
// The first check runs at once, so a page that reconnects catches up with
// anything that changed while it was away. The stream ends when its context
// is canceled, when the browser goes away, or after liveLifetime.
//
// Parameters:
//   - ctx: The stream's context.
//   - writer: The response body.
func (stream *clipStream) run(ctx context.Context, writer *bufio.Writer) {
	ticker := time.NewTicker(liveTick)
	defer ticker.Stop()

	lifetime := time.NewTimer(liveLifetime)
	defer lifetime.Stop()

	_, err := writer.WriteString(liveRetry)

	for err == nil {
		err = stream.send(ctx, writer)
		if err != nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-lifetime.C:
			return
		case <-ticker.C:
		}
	}
}

// send writes the region of every clip that changed, then a ping, and
// flushes them.
//
// Parameters:
//   - ctx: The stream's context.
//   - writer: The response body.
//
// Returns:
//   - err: Non-nil when the browser went away.
func (stream *clipStream) send(ctx context.Context, writer *bufio.Writer) error {
	for _, id := range stream.ids {
		err := stream.sendChanged(ctx, writer, id)
		if err != nil {
			return fmt.Errorf("send clip %s: %w", id, err)
		}
	}

	err := writeEvent(writer, liveEventPing, livePingData)
	if err != nil {
		return fmt.Errorf("send ping: %w", err)
	}

	err = writer.Flush()
	if err != nil {
		return fmt.Errorf("flush clip stream: %w", err)
	}

	return nil
}

// sendChanged writes a clip's region when it differs from the one last sent.
//
// A clip that is gone, or whose region cannot be rendered, is skipped, and
// the next check tries it again.
//
// Parameters:
//   - ctx: The stream's context.
//   - writer: The response body.
//   - id: The clip.
//
// Returns:
//   - err: Non-nil when the event cannot be written.
func (stream *clipStream) sendChanged(ctx context.Context, writer *bufio.Writer, id string) error {
	job := stream.lookup(ctx, id)
	if job == nil {
		return nil
	}

	html, err := stream.render(ctx, job)
	if err != nil || html == stream.sent[id] {
		return nil //nolint:nilerr // A region that fails to render is tried again next check.
	}

	// Two strings always encode.
	payload, _ := json.Marshal(liveUpdate{ID: id, HTML: html})

	err = writeEvent(writer, liveEventClip, string(payload))
	if err != nil {
		return fmt.Errorf("send update: %w", err)
	}

	stream.sent[id] = html

	return nil
}

// writeEvent writes one server-sent event.
//
// Parameters:
//   - writer: The response body.
//   - name: The event's name.
//   - data: The event's body, on one line.
//
// Returns:
//   - err: Non-nil when the event cannot be written.
func writeEvent(writer *bufio.Writer, name, data string) error {
	_, err := writer.WriteString("event: " + name + "\ndata: " + data + "\n\n")
	if err != nil {
		return fmt.Errorf("write %s event: %w", name, err)
	}

	return nil
}

// liveIDs reads the clips a stream follows, without repeats, up to
// maxLiveClips.
//
// Parameters:
//   - raw: The ids query, separated by commas.
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
