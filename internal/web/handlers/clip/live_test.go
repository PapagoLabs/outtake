// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	storagemocks "github.com/PapagoLabs/outtake/internal/store/blob/mocks"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/routes"
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

// fakeClips stands in for the catalog and the card renderer behind a stream.
type fakeClips struct {
	// mu guards every field.
	mu sync.Mutex
	// jobs are the clips the catalog holds.
	jobs map[string]*clipdom.Job
	// failing are the clips whose regions fail to render.
	failing map[string]bool
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

// fail sets whether a clip's region fails to render.
//
// Parameters:
//   - id: The clip.
//   - failing: Whether rendering fails.
func (clips *fakeClips) fail(id string, failing bool) {
	clips.mu.Lock()
	defer clips.mu.Unlock()

	clips.failing[id] = failing
}

// lookup finds a clip, as the catalog does.
//
// Parameters:
//   - id: The clip.
//
// Returns:
//   - job: A copy of the clip, nil when it is gone.
func (clips *fakeClips) lookup(_ context.Context, id string) *clipdom.Job {
	clips.mu.Lock()
	defer clips.mu.Unlock()

	return clips.jobs[id].Clone()
}

// render writes a region from the clip's status and progress.
//
// Parameters:
//   - job: The clip.
//
// Returns:
//   - html: The region.
//   - err: Non-nil for a clip marked as failing.
func (clips *fakeClips) render(_ context.Context, job *clipdom.Job) (string, error) {
	clips.mu.Lock()
	defer clips.mu.Unlock()

	if clips.failing[job.ID] {
		return "", errWriteFailed
	}

	return string(job.Status) + " " + strings.Repeat("|", job.Progress), nil
}

// set changes a clip's progress.
//
// Parameters:
//   - id: The clip.
//   - percent: Its new progress.
func (clips *fakeClips) set(id string, percent int) {
	clips.mu.Lock()
	defer clips.mu.Unlock()

	clips.jobs[id].Progress = percent
}

// parseEvents splits a stream's text into its events, leaving out the retry
// line.
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

// clipIDs returns the clips a list of events updated, in order.
//
// Parameters:
//   - t: The test reading the events.
//   - events: Events a stream wrote.
//
// Returns:
//   - ids: The id of each clip event.
func clipIDs(t *testing.T, events []streamedEvent) []string {
	t.Helper()

	ids := make([]string, 0, len(events))

	for _, event := range events {
		if event.name != liveEventClip {
			continue
		}

		var update liveUpdate

		require.NoError(t, json.Unmarshal([]byte(event.data), &update))

		ids = append(ids, update.ID)
	}

	return ids
}

// rendering returns a clip that is rendering.
//
// Parameters:
//   - id: The clip.
//
// Returns:
//   - job: The clip.
func rendering(id string) *clipdom.Job {
	job := testClipJob(id, clipdom.TypeClip)

	job.Status = clipdom.StatusProcessing

	return job
}

// newFakeStream builds a stream over fake clips.
//
// Parameters:
//   - clips: The catalog and renderer the stream reads.
//   - ids: The clips it follows.
//
// Returns:
//   - stream: The stream.
func newFakeStream(clips *fakeClips, ids ...string) *clipStream {
	return &clipStream{
		lookup: clips.lookup,
		render: clips.render,
		sent:   make(map[string]string),
		ids:    ids,
	}
}

func TestLiveIDs(t *testing.T) {
	t.Parallel()

	many := make([]string, 0, maxLiveClips+5)
	for index := range maxLiveClips + 5 {
		many = append(many, "clip-"+strings.Repeat("x", index))
	}

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "none", raw: "", want: []string{}},
		{name: "one", raw: "a", want: []string{"a"}},
		{name: "keeps the order", raw: "b,a", want: []string{"b", "a"}},
		{name: "drops repeats and blanks", raw: " a,,a, b ,", want: []string{"a", "b"}},
		{name: "caps the count", raw: strings.Join(many, ","), want: many[:maxLiveClips]},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, liveIDs(test.raw))
		})
	}
}

// TestClipStreamSendsEachChange covers what a stream writes on each check.
//
// The first check sends every clip it can find. Later checks send only a
// clip whose region changed, and every check ends with a ping. A region that
// fails to render is tried again on the next check.
func TestClipStreamSendsEachChange(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		clips := &fakeClips{
			mu:      sync.Mutex{},
			jobs:    map[string]*clipdom.Job{"a": rendering("a"), "b": rendering("b")},
			failing: map[string]bool{},
		}

		var buffer streamBuffer

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})

		go func() {
			defer close(done)

			newFakeStream(clips, "a", "b", "gone").run(ctx, bufio.NewWriter(&buffer))
		}()

		synctest.Wait()

		first := buffer.take()
		assert.Equal(
			t,
			[]string{"a", "b"},
			clipIDs(t, first),
			"the first check sends every clip it finds",
		)
		assert.Equal(t, liveEventPing, first[len(first)-1].name)

		time.Sleep(liveTick)
		synctest.Wait()

		quiet := buffer.take()
		assert.Empty(t, clipIDs(t, quiet), "nothing changed")
		assert.Len(t, quiet, 1, "the check still pings")

		clips.set("b", 10)
		clips.fail("b", true)
		time.Sleep(liveTick)
		synctest.Wait()

		assert.Empty(t, clipIDs(t, buffer.take()), "a region that fails to render is not sent")

		clips.fail("b", false)
		time.Sleep(liveTick)
		synctest.Wait()

		assert.Equal(t, []string{"b"}, clipIDs(t, buffer.take()), "the next check sends it")

		cancel()
		<-done
	})
}

// TestClipStreamEndsAfterItsLifetime covers the reconnect that takes the
// browser through the auth guard again.
func TestClipStreamEndsAfterItsLifetime(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		clips := &fakeClips{
			mu:      sync.Mutex{},
			jobs:    map[string]*clipdom.Job{"a": rendering("a")},
			failing: map[string]bool{},
		}

		var buffer streamBuffer

		done := make(chan struct{})

		go func() {
			defer close(done)

			newFakeStream(clips, "a").run(t.Context(), bufio.NewWriter(&buffer))
		}()

		time.Sleep(liveLifetime - time.Second)
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

// TestClipStreamEndsWhenTheBrowserLeaves covers a write that fails, as one
// to a closed connection does.
func TestClipStreamEndsWhenTheBrowserLeaves(t *testing.T) {
	t.Parallel()

	clips := &fakeClips{
		mu:      sync.Mutex{},
		jobs:    map[string]*clipdom.Job{"a": rendering("a")},
		failing: map[string]bool{},
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		newFakeStream(clips, "a").run(t.Context(), bufio.NewWriterSize(brokenWriter{}, 16))
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream kept writing to a closed connection")
	}
}

// TestStatusRendererReadsFilesOnlyWhenTheStateChanges covers the storage
// lookups behind a region.
//
// On S3 each lookup is a request. There is one per status and stage, not one
// per check.
func TestStatusRendererReadsFilesOnlyWhenTheStateChanges(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/live.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := rendering("clip-1")

	job.OutputPath = "/clips/clip-1.mp4"

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Exists(mock.Anything, job.OutputPath).Return(true).Twice()

	handler := New(
		queueForTest(t),
		store,
		blob.Paths{},
		db,
		&config.Config{MaxClipDur: time.Minute},
		nil,
	)
	stream := handler.newClipStream([]string{job.ID})

	for percent := range 3 {
		job.Progress = percent

		html, renderErr := stream.render(t.Context(), job)
		require.NoError(t, renderErr)
		assert.Contains(t, html, `id="clip-clip-1-status"`)
	}

	job.Status = clipdom.StatusCompleted

	_, err = stream.render(t.Context(), job)
	require.NoError(t, err)
}

func TestClipLiveAnswersNoContentWithoutClips(t *testing.T) {
	t.Parallel()

	handler, _ := clipPageHandler(t)

	app := fiber.New()
	app.Get(routes.PathClipLive, handler.ClipLive)

	for _, target := range []string{routes.PathClipLive, routes.PathClipLive + "?ids=,"} {
		answer := serveMethod(t, app, http.MethodGet, target, false, "", "")
		assert.Equal(t, fiber.StatusNoContent, answer.status, target)
	}
}

// TestClipLiveStreamsUntilItsStreamsClose covers a stream served over a real
// connection.
//
// It answers as an event stream and sends the clip's region and a ping. It ends
// once the handler closes its streams, as the server does before it shuts down.
func TestClipLiveStreamsUntilItsStreamsClose(t *testing.T) {
	t.Parallel()

	job := rendering("clip-1")

	db, err := database.New(t.TempDir() + "/live.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	handler := New(
		queueForTest(t, job),
		&blob.Storage{},
		blob.Paths{},
		db,
		&config.Config{MaxClipDur: time.Minute},
		nil,
	)

	app := fiber.New()
	app.Get(routes.PathClipLive, handler.ClipLive)

	addr := serveForStream(t, app)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"http://"+addr+routes.PathClipLive+"?ids="+job.ID, http.NoBody)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	t.Cleanup(func() { _ = response.Body.Close() })

	assert.Equal(t, contentTypeEventStream, response.Header.Get(fiber.HeaderContentType))
	assert.Equal(t, cacheNoStore, response.Header.Get(fiber.HeaderCacheControl))

	reader := bufio.NewReader(response.Body)
	first := readUntilPing(t, reader)

	assert.Equal(t, []string{job.ID}, clipIDs(t, first))

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

// serveForStream serves an app on a loopback listener.
//
// Parameters:
//   - t: The test that owns the server.
//   - app: The application to serve.
//
// Returns:
//   - addr: The server's host and port.
func serveForStream(t *testing.T, app *fiber.App) string {
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

		if strings.TrimSpace(line) == "data: "+livePingData {
			return parseEvents(text.String())
		}
	}
}
