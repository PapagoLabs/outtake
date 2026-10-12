// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package live

import (
	"bufio"
	"context"
	"fmt"
	"sync"
	"time"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/respond"
)

// Check returns the payloads that changed since its last call.
//
// Each payload is one JSON object on one line. A check is called from one
// goroutine at a time.
type Check = func(ctx context.Context) []string

// Source starts a check from the query value naming what to follow.
//
// The scope is whose clips and previews the stream's caller may see. It
// returns nil when the value names nothing it can follow.
type Source = func(value string, scope clip.Scope) Check

// Feed is one kind of update a stream can carry.
type Feed struct {
	// Name is both the query parameter that names what to follow and the
	// event name each payload is sent under.
	Name string
	// Start starts the feed's check for one stream.
	Start Source
}

// Handler serves the page event stream.
type Handler struct {
	// streamsDone closes when the server shuts down, which ends every stream,
	// because the server waits for open connections.
	streamsDone chan struct{}
	// feeds are the kinds of update a stream can carry, in the order each
	// check sends them.
	feeds []Feed
	// closeStreams closes streamsDone once.
	closeStreams sync.Once
}

// namedCheck is one feed's running check in a stream.
type namedCheck struct {
	// check reports what changed.
	check Check
	// name is the event name its payloads are sent under.
	name string
}

// stream sends the changes of every feed a page follows.
type stream struct {
	// checks are the feeds the page follows.
	checks []namedCheck
}

const (
	// tick is how often a stream checks its feeds.
	tick = time.Second
	// lifetime is how long one stream runs.
	//
	// The browser then reconnects through the auth guard, so a revoked
	// session or a logout ends it.
	lifetime = time.Minute
	// retry tells the browser to reconnect a second after a stream ends.
	retry = "retry: 1000\n\n"
	// eventPing names the event every check ends with, which tells the page
	// the stream still delivers.
	eventPing = "ping"
	// pingData is the body of a ping, which must not be empty for the browser
	// to deliver it.
	pingData = "{}"
	// contentTypeEventStream is the media type of a server-sent event stream.
	contentTypeEventStream = "text/event-stream"
	// headerAccelBuffering asks a reverse proxy not to buffer the stream.
	headerAccelBuffering = "X-Accel-Buffering"
	// cacheNoStore keeps the stream out of every cache.
	cacheNoStore = "no-store"
)

// New creates the stream handler.
//
// Parameters:
//   - feeds: The kinds of update a stream can carry.
//
// Returns:
//   - handler: A handler serving the stream.
func New(feeds ...Feed) *Handler {
	return &Handler{
		streamsDone:  make(chan struct{}),
		feeds:        feeds,
		closeStreams: sync.Once{},
	}
}

// CloseStreams ends every open stream.
//
// The server can then shut down without waiting on them. Later calls do
// nothing.
func (handler *Handler) CloseStreams() {
	handler.closeStreams.Do(func() {
		close(handler.streamsDone)
	})
}

// Stream sends the changes of every feed the page names in the query.
//
// Each change is sent when it happens, checked once a second, and every check
// ends with a ping. A request naming nothing to follow answers 204, which
// tells the browser not to reconnect.
//
// Parameters:
//   - ctx: Request naming what to follow.
//
// Returns:
//   - err: Non-nil when the response cannot be started.
func (handler *Handler) Stream(ctx fiber.Ctx) error {
	checks := make([]namedCheck, 0, len(handler.feeds))
	scope := middleware.ClipScope(ctx)

	for _, feed := range handler.feeds {
		value := ctx.Query(feed.Name)
		if value == "" {
			continue
		}

		if check := feed.Start(value, scope); check != nil {
			checks = append(checks, namedCheck{check: check, name: feed.Name})
		}
	}

	if len(checks) == 0 {
		return respond.SendStatusCode(ctx, fiber.StatusNoContent)
	}

	ctx.Set(fiber.HeaderContentType, contentTypeEventStream)
	ctx.Set(fiber.HeaderCacheControl, cacheNoStore)
	ctx.Set(headerAccelBuffering, "no")

	page := &stream{checks: checks}

	err := ctx.SendStreamWriter(func(writer *bufio.Writer) {
		streamCtx, cancel := handler.streamContext()
		defer cancel()

		page.run(streamCtx, writer)
	})
	if err != nil {
		return fmt.Errorf("start live stream: %w", err)
	}

	return nil
}

// streamContext returns the context a stream runs in.
//
// It is canceled when the server shuts down. The request's own context ends
// when the handler returns, before the stream starts.
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

// run sends the feeds' changes until the stream ends.
//
// The first check runs at once, so a page that reconnects catches up with
// anything that changed while it was away. The stream ends when its context
// is canceled, when the browser goes away, or after its lifetime.
//
// Parameters:
//   - ctx: The stream's context.
//   - writer: The response body.
func (page *stream) run(ctx context.Context, writer *bufio.Writer) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	ends := time.NewTimer(lifetime)
	defer ends.Stop()

	_, err := writer.WriteString(retry)

	for err == nil {
		err = page.send(ctx, writer)
		if err != nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ends.C:
			return
		case <-ticker.C:
		}
	}
}

// send writes every change the feeds report, then a ping, and flushes them.
//
// Parameters:
//   - ctx: The stream's context.
//   - writer: The response body.
//
// Returns:
//   - err: Non-nil when the browser went away.
func (page *stream) send(ctx context.Context, writer *bufio.Writer) error {
	for _, feed := range page.checks {
		err := sendFeed(ctx, writer, feed)
		if err != nil {
			return fmt.Errorf("send feeds: %w", err)
		}
	}

	err := writeEvent(writer, eventPing, pingData)
	if err != nil {
		return fmt.Errorf("send ping: %w", err)
	}

	err = writer.Flush()
	if err != nil {
		return fmt.Errorf("flush live stream: %w", err)
	}

	return nil
}

// sendFeed writes every change one feed reports.
//
// Parameters:
//   - ctx: The stream's context.
//   - writer: The response body.
//   - feed: The feed to check.
//
// Returns:
//   - err: Non-nil when the browser went away.
func sendFeed(ctx context.Context, writer *bufio.Writer, feed namedCheck) error {
	for _, payload := range feed.check(ctx) {
		err := writeEvent(writer, feed.name, payload)
		if err != nil {
			return fmt.Errorf("send %s update: %w", feed.name, err)
		}
	}

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
