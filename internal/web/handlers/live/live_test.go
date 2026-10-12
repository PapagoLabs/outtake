// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package live

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"
)

// streamedEvent is one server-sent event a test read back.
type streamedEvent struct {
	// name is the event's name.
	name string
	// data is the event's body.
	data string
}

// streamBuffer collects what a stream writes, for a test reading it from
// another goroutine.
type streamBuffer struct {
	// mu guards written.
	mu sync.Mutex
	// written is everything written so far.
	written bytes.Buffer
}

// queuedFeed is a check that reports whatever the test queued since the last
// call.
type queuedFeed struct {
	// mu guards pending.
	mu sync.Mutex
	// pending are the payloads the next check reports.
	pending []string
}

// brokenWriter fails every write, as a closed connection does.
type brokenWriter struct{}

// errWriteFailed stands in for a browser that went away.
var errWriteFailed = errors.New("connection closed")

// Write implements [io.Writer] and always fails.
//
// Returns:
//   - written: Always 0.
//   - err: Always errWriteFailed.
func (brokenWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

// check reports and clears the queued payloads.
//
// Returns:
//   - payloads: What was queued since the last check.
func (feed *queuedFeed) check(context.Context) []string {
	feed.mu.Lock()
	defer feed.mu.Unlock()

	payloads := feed.pending

	feed.pending = nil

	return payloads
}

// queue adds payloads for the next check.
//
// Parameters:
//   - payloads: What the next check reports.
func (feed *queuedFeed) queue(payloads ...string) {
	feed.mu.Lock()
	defer feed.mu.Unlock()

	feed.pending = append(feed.pending, payloads...)
}

// Write implements [io.Writer].
//
// Parameters:
//   - chunk: Bytes the stream wrote.
//
// Returns:
//   - written: Always len(chunk).
//   - err: Always nil.
func (buffer *streamBuffer) Write(chunk []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	//nolint:wrapcheck // bytes.Buffer never fails a write.
	return buffer.written.Write(chunk)
}

// take returns the events written since the last take.
//
// Returns:
//   - events: The events, in the order written.
func (buffer *streamBuffer) take() []streamedEvent {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	events := parseEvents(buffer.written.String())
	buffer.written.Reset()

	return events
}

// parseEvents splits a stream's text into its events.
//
// The retry line is left out.
//
// Parameters:
//   - text: What the stream wrote.
//
// Returns:
//   - events: The named events, in order.
func parseEvents(text string) []streamedEvent {
	var events []streamedEvent

	for block := range strings.SplitSeq(text, "\n\n") {
		var event streamedEvent

		for line := range strings.SplitSeq(block, "\n") {
			if name, ok := strings.CutPrefix(line, "event: "); ok {
				event.name = name
			}

			if data, ok := strings.CutPrefix(line, "data: "); ok {
				event.data = data
			}
		}

		if event.name != "" {
			events = append(events, event)
		}
	}

	return events
}

// TestStreamSendsEachFeedsChanges covers what a stream writes on each check.
//
// Every payload a feed reports goes out under the feed's name, in feed order,
// and every check ends with a ping, whether or not anything changed.
func TestStreamSendsEachFeedsChanges(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		clips := &queuedFeed{mu: sync.Mutex{}, pending: []string{`{"id":"a"}`}}
		playback := &queuedFeed{mu: sync.Mutex{}, pending: []string{`{"id":"5"}`}}

		page := &stream{checks: []namedCheck{
			{check: clips.check, name: "clip"},
			{check: playback.check, name: "playback"},
		}}

		var buffer streamBuffer

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})

		go func() {
			defer close(done)

			page.run(ctx, bufio.NewWriter(&buffer))
		}()

		synctest.Wait()

		assert.Equal(t, []streamedEvent{
			{name: "clip", data: `{"id":"a"}`},
			{name: "playback", data: `{"id":"5"}`},
			{name: eventPing, data: pingData},
		}, buffer.take(), "the first check runs at once")

		time.Sleep(tick)
		synctest.Wait()

		assert.Equal(t, []streamedEvent{{name: eventPing, data: pingData}}, buffer.take(),
			"a check with no change still pings")

		clips.queue(`{"id":"b"}`)
		time.Sleep(tick)
		synctest.Wait()

		assert.Equal(t, []streamedEvent{
			{name: "clip", data: `{"id":"b"}`},
			{name: eventPing, data: pingData},
		}, buffer.take())

		cancel()
		<-done
	})
}

// TestStreamEndsAfterItsLifetime covers the reconnect that takes the browser
// through the auth guard again.
func TestStreamEndsAfterItsLifetime(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		page := &stream{checks: []namedCheck{{check: (&queuedFeed{}).check, name: "clip"}}}

		var buffer streamBuffer

		done := make(chan struct{})

		go func() {
			defer close(done)

			page.run(t.Context(), bufio.NewWriter(&buffer))
		}()

		time.Sleep(lifetime - time.Second)
		synctest.Wait()

		select {
		case <-done:
			t.Fatal("the stream ended before its lifetime")
		default:
		}

		time.Sleep(time.Second)
		synctest.Wait()

		select {
		case <-done:
		default:
			t.Fatal("the stream outlived its lifetime")
		}
	})
}

// TestStreamEndsWhenTheBrowserLeaves covers a write that fails, as one to a
// closed connection does.
func TestStreamEndsWhenTheBrowserLeaves(t *testing.T) {
	t.Parallel()

	page := &stream{checks: []namedCheck{{check: (&queuedFeed{}).check, name: "clip"}}}
	done := make(chan struct{})

	go func() {
		defer close(done)

		page.run(t.Context(), bufio.NewWriterSize(brokenWriter{}, 16))
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream kept writing to a closed connection")
	}
}

// TestStreamAnswersNoContentWhenNothingIsFollowed covers a page that names
// nothing a feed can follow.
//
// A 204 tells the browser not to reconnect.
func TestStreamAnswersNoContentWhenNothingIsFollowed(t *testing.T) {
	t.Parallel()

	refusing := Feed{Name: "clip", Start: func(string) Check { return nil }}

	app := fiber.New()
	app.Get("/live", New(refusing).Stream)

	for _, target := range []string{"/live", "/live?clip=unknown", "/live?other=1"} {
		response, err := app.Test(
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil),
		)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())

		assert.Equal(t, fiber.StatusNoContent, response.StatusCode, target)
	}
}

// TestStreamServesUntilItsStreamsClose covers a stream over a real connection.
//
// It answers as an event stream, sends what its feed reports and a ping, and
// ends once the handler closes its streams, as the server does before it
// shuts down.
func TestStreamServesUntilItsStreamsClose(t *testing.T) {
	t.Parallel()

	var started string

	feed := Feed{Name: "preview", Start: func(value string) Check {
		started = value

		return (&queuedFeed{mu: sync.Mutex{}, pending: []string{`{"id":"p1"}`}}).check
	}}

	handler := New(feed)

	app := fiber.New()
	app.Get("/live", handler.Stream)

	addr := serve(t, app)

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"http://"+addr+"/live?preview=p1",
		http.NoBody,
	)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	t.Cleanup(func() { _ = response.Body.Close() })

	assert.Equal(t, contentTypeEventStream, response.Header.Get(fiber.HeaderContentType))
	assert.Equal(t, cacheNoStore, response.Header.Get(fiber.HeaderCacheControl))
	assert.Equal(t, "p1", started, "the feed starts from the query value")

	reader := bufio.NewReader(response.Body)

	assert.Equal(t, []streamedEvent{
		{name: "preview", data: `{"id":"p1"}`},
		{name: eventPing, data: pingData},
	}, readUntilPing(t, reader))

	handler.CloseStreams()
	handler.CloseStreams()

	ended := make(chan error, 1)

	go func() {
		_, readErr := reader.ReadString(0)
		ended <- readErr
	}()

	select {
	case readErr := <-ended:
		require.Error(t, readErr, "the body ends")
	case <-time.After(5 * time.Second):
		t.Fatal("the stream outlived CloseStreams")
	}
}

// serve serves an app on a loopback listener.
//
// Parameters:
//   - t: The test that owns the server.
//   - app: The application to serve.
//
// Returns:
//   - addr: The server's host and port.
func serve(t *testing.T, app *fiber.App) string {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	served := make(chan error, 1)

	go func() {
		served <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()

	t.Cleanup(func() {
		_ = app.ShutdownWithTimeout(time.Second)

		<-served
	})

	return listener.Addr().String()
}

// readUntilPing reads a stream's events up to and including its first ping.
//
// Parameters:
//   - t: The test reading the stream.
//   - reader: The stream's body.
//
// Returns:
//   - events: The events read.
func readUntilPing(t *testing.T, reader *bufio.Reader) []streamedEvent {
	t.Helper()

	var text strings.Builder

	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)

		text.WriteString(line)

		if line == "data: "+pingData+"\n" {
			text.WriteString("\n")

			return parseEvents(text.String())
		}
	}
}
