// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/playback"
	clippreview "github.com/PapagoLabs/outtake/internal/clip/preview"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/plex/library/mocks"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/respond/respondtest"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

type statusResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	URL      string `json:"url"`
	Error    string `json:"error"`
	Format   string `json:"format"`
}

type errorResponse struct {
	Error   api.ErrorCode `json:"error"`
	Message string        `json:"message"`
}

// previewResponse is a decoded preview handler response.
type previewResponse struct {
	status   int
	location string
	failure  errorResponse
	// flash is the failure a form post left for the page it returns to.
	flash string
}

func previewTestHandler(t *testing.T, limit int) (*Handler, *blob.Storage) {
	t.Helper()

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	service := clippreview.New(limit, store, store.Paths, nil)

	return New(
		service,
		&config.Config{MaxConcurrentPreviews: limit},
		nil,
		nil,
	), store
}

func getPreviewStatus(
	t *testing.T,
	handler *Handler,
	id string,
) (int, statusResponse) {
	t.Helper()

	app := fiber.New()
	app.Get("/api/clips/preview/:id", handler.PreviewStatus)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/clips/preview/"+id, nil,
	))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	var body statusResponse

	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	}

	return resp.StatusCode, body
}

func getPreviewFile(t *testing.T, handler *Handler, id string) int {
	t.Helper()

	app := fiber.New()
	app.Get("/previews/:id", handler.PreviewFile)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/previews/"+id, nil,
	))
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp.StatusCode
}

func awaitTerminal(t *testing.T, service *clippreview.Service, previewID string) clippreview.View {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		view, ok := service.Status(previewID)
		require.True(t, ok, "the preview should still be registered")

		if view.Done() {
			return view
		}

		if time.Now().After(deadline) {
			t.Fatalf(
				"preview never reached a terminal status, last was %q at %d%%",
				view.Status,
				view.Progress,
			)
		}

		time.Sleep(time.Millisecond)
	}
}

func cancelPreview(t *testing.T, handler *Handler, id string) int {
	t.Helper()

	app := fiber.New()
	app.Delete("/api/clips/preview/:id", handler.CancelPreview)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodDelete, "/api/clips/preview/"+id, nil,
	))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp.StatusCode
}

func TestPreviewStatusReportsAQueuedRender(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 2)

	started := make(chan struct{})
	release := make(chan struct{})

	require.Equal(
		t,
		clippreview.AdmittedRender,
		handler.previews.Submit(t.Context(), "p1", func(context.Context) error {
			close(started)
			<-release

			return nil
		}),
	)

	<-started

	view, ok := handler.previews.Status("p1")
	require.True(t, ok)
	assert.Equal(t, "processing", string(view.Status))
	assert.Empty(t, view.Error)

	close(release)
}

func TestPreviewStatusReportsARenderWaitingForASlot(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 2)

	var (
		mu      sync.Mutex
		holders int
	)

	release := make(chan struct{})

	occupy := func() {
		mu.Lock()

		holders++

		acquired, err := handler.previews.Acquire(t.Context(), time.Second)
		mu.Unlock()
		require.NoError(t, err)

		<-release

		acquired()
	}

	var occupying sync.WaitGroup

	for range 2 {
		occupy := occupy
		occupying.Go(occupy)
	}

	deadline := time.Now().Add(5 * time.Second)

	for {
		mu.Lock()

		held := holders == 2
		mu.Unlock()

		if held {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("the preview slots were never both taken")
		}

		time.Sleep(time.Millisecond)
	}

	waiting := make(chan struct{})

	require.Equal(
		t,
		clippreview.AdmittedRender,
		handler.previews.Submit(
			t.Context(),
			"queued",
			func(ctx context.Context) error {
				close(waiting)
				<-ctx.Done()

				return ctx.Err()
			},
		),
	)

	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued render never started waiting for a slot")
	}

	status, body := getPreviewStatus(t, handler, "queued")
	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, "processing", body.Status, "a render waiting for a slot is still processing")

	close(release)
	occupying.Wait()

	require.True(t, handler.previews.Cancel("queued"))
	awaitTerminal(t, handler.previews, "queued")
}

func TestPreviewStatusSeesACachedPreview(t *testing.T) {
	t.Parallel()

	handler, store := previewTestHandler(t, 2)

	require.NoError(t, os.MkdirAll(filepath.Join(store.BasePath(), "previews"), 0o750))
	require.NoError(t, os.WriteFile(store.PreviewPath("cached"), []byte("x"), 0o600))

	handler.previews.Remember(t.Context(), "cached")

	status, body := getPreviewStatus(t, handler, "cached")
	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, "completed", body.Status)
	assert.Equal(t, "/previews/cached", body.URL)
}

func TestPreviewStatusOmitsURLUntilPublished(t *testing.T) {
	t.Parallel()

	handler, store := previewTestHandler(t, 2)

	handler.previews.Submit(t.Context(), "p1", func(context.Context) error { return nil })

	awaitTerminal(t, handler.previews, "p1")

	_, body := getPreviewStatus(t, handler, "p1")
	assert.Equal(t, "completed", body.Status)
	assert.Empty(t, body.URL, "a preview with no published file must not be offered a URL")

	require.NoError(t, os.WriteFile(store.PreviewPath("p1"), []byte("x"), 0o600))

	_, body = getPreviewStatus(t, handler, "p1")
	assert.Equal(t, "/previews/p1", body.URL)
}

func TestPreviewStatusReportsAFailedRender(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 2)

	handler.previews.Submit(
		t.Context(),
		"p1",
		func(context.Context) error { return assert.AnError },
	)

	view := awaitTerminal(t, handler.previews, "p1")

	_, body := getPreviewStatus(t, handler, "p1")
	assert.Equal(t, "failed", body.Status)
	assert.Equal(t, assert.AnError.Error(), body.Error)
	assert.Empty(t, body.URL, "a failed render published nothing to serve")
	assert.Equal(t, "failed", string(view.Status))
}

func TestPreviewStatusUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 2)

	status, _ := getPreviewStatus(t, handler, "never-rendered")

	assert.Equal(t, fiber.StatusNotFound, status)
}

func TestCancelPreviewUnknownIDIsNotFound(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 2)

	assert.Equal(t, fiber.StatusNotFound, cancelPreview(t, handler, "never-rendered"))
}

func TestCancelPreviewStopsARunningRender(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 2)

	running := make(chan struct{})
	finished := make(chan struct{})

	handler.previews.Submit(t.Context(), "p1", func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		close(finished)

		return ctx.Err()
	})

	<-running

	assert.Equal(t, fiber.StatusOK, cancelPreview(t, handler, "p1"))

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("canceling did not reach the render")
	}

	view := awaitTerminal(t, handler.previews, "p1")
	assert.Equal(t, "canceled", string(view.Status))
}

func TestCancelPreviewRejectsAFinishedRender(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 2)

	handler.previews.Submit(t.Context(), "p1", func(context.Context) error { return nil })

	awaitTerminal(t, handler.previews, "p1")

	assert.Equal(t, fiber.StatusConflict, cancelPreview(t, handler, "p1"))
}

func TestPreviewHandlerTakesTheConfiguredLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "the configured limit is used", limit: 4, want: 4},
		{name: "a zero limit falls back to one", limit: 0, want: 1},
		{name: "a negative limit falls back to one", limit: -2, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := previewTestHandler(t, test.limit)

			assert.Equal(t, test.want, handler.previews.Slots())
		})
	}
}

func TestPreviewFileServesAValidID(t *testing.T) {
	t.Parallel()

	const name = "1f0c3a52-8b6d-4e21-9a77-2c5d8e4f1b03"

	handler, store := previewTestHandler(t, 1)
	seedPreviewID(t, store, name)

	assert.Equal(t, fiber.StatusOK, getPreviewFile(t, handler, name))
}

func TestPreviewFileRejectsUnsafeIDs(t *testing.T) {
	t.Parallel()

	ids := []string{
		"..",
		"a.b",
		"a%00b",
		"a%20b",
		"..%2f..%2fetc%2fpasswd",
		"%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"%252e%252e%252fsecret",
		"..%5c..%5csecret",
	}

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			handler, store := previewTestHandler(t, 1)
			seedPreviewID(t, store, id)

			assert.Equal(t, fiber.StatusNotFound, getPreviewFile(t, handler, id),
				"the guard must reject the id even though the file exists")
		})
	}
}

func TestPreviewFileRejectsBeforeRouting(t *testing.T) {
	t.Parallel()

	ids := []string{
		"a/b",
		"../secret",
		"",
	}

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			handler, _ := previewTestHandler(t, 1)

			assert.Equal(t, fiber.StatusNotFound, getPreviewFile(t, handler, id))
		})
	}
}

func TestPreviewFileRejectsOverlongID(t *testing.T) {
	t.Parallel()

	handler, store := previewTestHandler(t, 1)

	seedPreviewID(t, store, longPreviewID(65))

	assert.Equal(t, fiber.StatusNotFound, getPreviewFile(t, handler, longPreviewID(65)))
}

func longPreviewID(n int) string {
	return strings.Repeat("a", n)
}

func seedPreviewID(t *testing.T, store *blob.Storage, id string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(
		filepath.Join(store.BasePath(), "previews"), 0o750,
	))
	require.NoError(t, os.WriteFile(store.PreviewPath(id), []byte("preview"), 0o600))
}

// stubFFmpeg returns a runner whose ffmpeg writes the file it was asked for
// and whose ffprobe reports a plain high-definition source.
//
// Parameters:
//   - t: The test the runner belongs to.
//
// Returns:
//   - runner: An ffmpeg runner backed by fakes.
func stubFFmpeg(t *testing.T) *ffmpeg.ExecFFmpeg {
	t.Helper()

	const probeJSON = `{"format":{"duration":"120.0","bit_rate":"8000","format_name":"matroska"},` +
		`"streams":[{"index":0,"codec_type":"video","codec_name":"h264","width":1920,` +
		`"height":1080,"color_transfer":"bt709"},{"index":1,"codec_type":"audio",` +
		`"codec_name":"aac","channels":2},{"index":2,"codec_type":"audio",` +
		`"codec_name":"aac","channels":2},{"index":3,"codec_type":"audio",` +
		`"codec_name":"ac3","channels":6}]}` + "\n"

	return ffmpeg.NewExecFFmpeg(
		ffmpegtest.Install(t, ffmpegtest.Stub{Output: "encoded"}),
		ffmpegtest.Install(t, ffmpegtest.Stub{Stdout: probeJSON}),
	)
}

// sourceFile writes a media file a preview can be cut from.
//
// Parameters:
//   - t: The test the file belongs to.
//
// Returns:
//   - path: The path of the written file.
func sourceFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, make([]byte, 32), 0o600))

	return path
}

// previewHandler builds a preview handler over a throwaway store.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - limit: Maximum simultaneous renders.
//   - env: Environment the media resolver runs under.
//   - selected: Plex server selection the resolver uses off the e2e path.
//
// Returns:
//   - handler: The preview handler under test.
//   - store: The storage previews are published into.
func previewHandler(
	t *testing.T,
	limit int,
	env string,
	selected library.ServerSelection,
) (*Handler, *blob.Storage) {
	t.Helper()

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	cfg := &config.Config{MaxConcurrentPreviews: limit, Env: env}
	runner := stubFFmpeg(t)
	service := clippreview.New(limit, store, store.Paths, runner)
	sources := library.NewMediaSource(cfg, selected, runner)

	return New(service, cfg, nil, sources), store
}

// e2ePreviewHandler builds a handler whose media ids name local files, which is
// how the e2e environment resolves a preview's source.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - limit: Maximum simultaneous renders.
//
// Returns:
//   - handler: A preview handler that can render.
//   - store: The storage the handler publishes previews into.
func e2ePreviewHandler(t *testing.T, limit int) (*Handler, *blob.Storage) {
	t.Helper()

	return previewHandler(t, limit, "e2e", nil)
}

// unselectedPreviewHandler builds a handler with no Plex server selected, so
// every media id that is not a local file fails to resolve.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - limit: Maximum simultaneous renders.
//   - env: Environment the media resolver runs under.
//
// Returns:
//   - handler: A preview handler that cannot resolve a Plex media id.
func unselectedPreviewHandler(t *testing.T, limit int, env string) *Handler {
	t.Helper()

	selection := mocks.NewMockServerSelection(t)
	selection.EXPECT().Client().Return(nil, plex.Server{}, false).Maybe()

	handler, _ := previewHandler(t, limit, env, selection)

	return handler
}

// pmsPreviewHandler builds a handler that resolves a media id through a Plex
// Media Server stub reporting the given file path.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - limit: Maximum simultaneous renders.
//   - mediaFile: Path the stub reports for the media item.
//
// Returns:
//   - handler: A preview handler that resolves against the stub server.
func pmsPreviewHandler(t *testing.T, limit int, mediaFile string) *Handler {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			fmt.Fprintf(writer,
				`{"MediaContainer":{"Metadata":[{"Media":[{"Part":[{"file":%q}]}]}]}}`,
				mediaFile,
			)
		},
	))
	t.Cleanup(server.Close)

	host, rawPort, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)

	selection := mocks.NewMockServerSelection(t)
	selection.EXPECT().Client().Return(
		plex.NewClient(plex.ClientConfig{Token: "preview-token"}),
		plex.Server{Scheme: "http", Address: host, Port: port},
		true,
	).Maybe()

	handler, _ := previewHandler(t, limit, "", selection)

	return handler
}

// postPreview submits a preview request and decodes whatever came back.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: Preview handler under test.
//   - contentType: Content type the request is sent as.
//   - body: Encoded request body.
//
// Returns:
//   - resp: The handler's decoded response.
func postPreview(
	t *testing.T,
	handler *Handler,
	contentType string,
	body io.Reader,
) previewResponse {
	t.Helper()

	app := fiber.New()
	respondtest.Sessions(t, app)
	app.Post("/api/clips/preview", handler.Preview)

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/preview", body,
	)
	req.Header.Set(fiber.HeaderContentType, contentType)

	raw, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = raw.Body.Close() }()

	resp := previewResponse{
		status:   raw.StatusCode,
		location: raw.Header.Get(fiber.HeaderLocation),
		flash:    respondtest.Flash(t, app, raw).Message,
	}

	if resp.status >= fiber.StatusBadRequest {
		require.NoError(t, json.NewDecoder(raw.Body).Decode(&resp.failure))
	}

	return resp
}

// postPreviewForm submits a browser form post for a preview.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: Preview handler under test.
//   - form: Form fields the export form submitted.
//
// Returns:
//   - resp: The handler's decoded response.
func postPreviewForm(t *testing.T, handler *Handler, form url.Values) previewResponse {
	t.Helper()

	return postPreview(
		t,
		handler,
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
}

// previewWindowForm is the export form's selection for one preview.
//
// Parameters:
//   - mediaID: Media the selection is cut from.
//   - start: Where the selection begins in the source, in seconds.
//   - end: Where the selection ends in the source, in seconds.
//
// Returns:
//   - form: The form fields a browser would submit.
func previewWindowForm(mediaID string, start, end int) url.Values {
	return url.Values{
		"mediaId":   []string{mediaID},
		"startTime": []string{fmt.Sprintf("00:00:%02d", start)},
		"endTime":   []string{fmt.Sprintf("00:00:%02d", end)},
		"name":      []string{"Opening"},
		"clipType":  []string{"clip"},
		"quality":   []string{"medium"},
	}
}

// previewID reads the preview id out of the redirect.
//
// Parameters:
//   - t: The test the response belongs to.
//
// Returns:
//   - previewID: The id the media item page polls.
func (resp previewResponse) previewID(t *testing.T) string {
	t.Helper()

	_, query := resp.redirect(t)

	previewID := query.Get(routes.QueryPreview)
	require.NotEmpty(t, previewID, "the redirect carries no preview id")

	return previewID
}

// redirect reads the redirect response apart.
//
// Parameters:
//   - t: The test the response belongs to.
//
// Returns:
//   - path: The page the response returns to.
//   - query: The query the response carries.
func (resp previewResponse) redirect(t *testing.T) (string, url.Values) {
	t.Helper()

	require.NotEmpty(t, resp.location, "the response is not a redirect")

	parsed, err := url.Parse(resp.location)
	require.NoError(t, err)

	return parsed.Path, parsed.Query()
}

// fillPreviewQueue registers enough renders that no further one is admitted.
//
// Parameters:
//   - t: The test the renders belong to.
//   - handler: Preview handler owning the service.
//   - slots: Number of render slots the service admits.
func fillPreviewQueue(t *testing.T, handler *Handler, slots int) {
	t.Helper()

	held := 4 * slots
	release := make(chan struct{})
	started := make(chan struct{}, held)

	t.Cleanup(func() { close(release) })

	for index := range held {
		id := "queued-" + strconv.Itoa(index)

		require.Equal(t, clippreview.AdmittedRender, handler.previews.Submit(
			t.Context(), id, func(ctx context.Context) error {
				started <- struct{}{}

				select {
				case <-release:
				case <-ctx.Done():
				}

				return ctx.Err()
			},
		))
	}

	for range held {
		<-started
	}
}

func TestPreviewStartsARender(t *testing.T) {
	t.Parallel()

	handler, store := e2ePreviewHandler(t, 2)

	mediaID := sourceFile(t)
	resp := postPreviewForm(t, handler, previewWindowForm(mediaID, 10, 20))

	path, query := resp.redirect(t)
	previewID := query.Get(routes.QueryPreview)

	assert.Equal(t, fiber.StatusSeeOther, resp.status)
	assert.Equal(t, "/media/item/"+mediaID, path, "the browser returns to the source page")
	require.True(t, clippreview.ValidID(previewID), "the redirect carries a usable preview id")
	assert.Equal(t, "10.000", query.Get(routes.QueryStart))
	assert.Equal(t, "20.000", query.Get(routes.QueryEnd))

	view := awaitTerminal(t, handler.previews, previewID)
	assert.Equal(t, "completed", string(view.Status))
	assert.Empty(t, view.Error)
	assert.FileExists(t, store.PreviewPath(previewID), "the finished render is published")

	status, body := getPreviewStatus(t, handler, previewID)
	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, routes.PathPreviewPrefix+previewID, body.URL)
	assert.Equal(t, "SDR · 1080p", body.Format,
		"the published preview is read for its badge, here from the probe stub")
}

// TestPreviewFollowsTheMaximumPreviewResolution covers the setting reaching a
// preview: the same selection asks for a new preview once the maximum preview
// resolution changes, rather than reusing one rendered under the old maximum.
func TestPreviewFollowsTheMaximumPreviewResolution(t *testing.T) {
	t.Parallel()

	db, err := database.New(filepath.Join(t.TempDir(), "settings.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	cfg := &config.Config{MaxConcurrentPreviews: 2, Env: "e2e"}
	runner := stubFFmpeg(t)
	handler := New(
		clippreview.New(2, store, store.Paths, runner),
		cfg,
		db,
		library.NewMediaSource(cfg, nil, runner),
	)

	form := previewWindowForm(sourceFile(t), 10, 20)

	_, before := postPreviewForm(t, handler, form).redirect(t)
	awaitTerminal(t, handler.previews, before.Get(routes.QueryPreview))

	require.NoError(t, playback.SaveMaxPreviewWidth(
		t.Context(), db, strconv.Itoa(clip.OutputWidth2160p),
	))

	_, after := postPreviewForm(t, handler, form).redirect(t)
	awaitTerminal(t, handler.previews, after.Get(routes.QueryPreview))

	assert.NotEqual(t, before.Get(routes.QueryPreview), after.Get(routes.QueryPreview))
}

func TestPreviewCarriesTheExportFormAcrossTheRedirect(t *testing.T) {
	t.Parallel()

	handler, _ := e2ePreviewHandler(t, 2)

	form := previewWindowForm(sourceFile(t), 0, 10)
	form.Set("width", "480")
	form.Set("fps", "12")
	form.Set("audioIndex", "2")
	form.Set("cropBlackBars", routes.FormChecked)

	_, query := postPreviewForm(t, handler, form).redirect(t)

	assert.Equal(t, "Opening", query.Get(routes.QueryExportName))
	assert.Equal(t, "clip", query.Get(routes.QueryExportType))
	assert.Equal(t, "medium", query.Get(routes.QueryQuality))
	assert.Equal(t, "480", query.Get(routes.QueryWidth))
	assert.Equal(t, "12", query.Get(routes.QueryFPS))
	assert.Equal(t, "2", query.Get(routes.QueryAudioIndex))
	assert.Equal(t, routes.FormChecked, query.Get(routes.QueryCropBlackBars))
}

func TestPreviewReusesAnAlreadyPublishedPreview(t *testing.T) {
	t.Parallel()

	handler, _ := e2ePreviewHandler(t, 2)

	form := previewWindowForm(sourceFile(t), 30, 40)

	first := postPreviewForm(t, handler, form)
	previewID := first.previewID(t)

	awaitTerminal(t, handler.previews, previewID)

	second := postPreviewForm(t, handler, form)

	firstPath, firstQuery := first.redirect(t)
	secondPath, secondQuery := second.redirect(t)

	assert.Equal(t, firstPath, secondPath)
	assert.Equal(t, firstQuery, secondQuery,
		"the same selection resolves to the same preview")
	assert.Equal(t, previewID, secondQuery.Get(routes.QueryPreview))

	view, ok := handler.previews.Status(previewID)
	require.True(t, ok,
		"a cached preview is recorded so the page polls a status rather than an unknown id")
	assert.Equal(t, "completed", string(view.Status))
}

func TestPreviewRejectsAnUnbindableRequest(t *testing.T) {
	t.Parallel()

	handler := unselectedPreviewHandler(t, 1, "")

	resp := postPreview(t, handler, "application/json", strings.NewReader(`{"mediaId":`))

	assert.Equal(t, fiber.StatusBadRequest, resp.status)

	assert.Equal(t, api.InvalidRequest, resp.failure.Error)
	assert.Equal(t, "The request body isn't valid JSON", resp.failure.Message)
}

func TestPreviewRejectsAMediaIDThatDoesNotResolve(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "never-written.mkv")

	ids := map[string]string{
		"an absent media id":           "",
		"an id no server holds":        "12345",
		"a local path that is no file": missing,
	}

	for name, mediaID := range ids {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler := unselectedPreviewHandler(t, 1, "e2e")
			body := fmt.Sprintf(`{"mediaId":%q}`, mediaID)

			resp := postPreview(t, handler, "application/json", strings.NewReader(body))

			assert.Equal(t, fiber.StatusBadRequest, resp.status)
			assert.Equal(t, api.MediaPathUnresolved, resp.failure.Error)
			assert.Equal(t, "Choose a Plex server under Servers first", resp.failure.Message)
		})
	}
}

func TestPreviewRejectsASourceItCannotRead(t *testing.T) {
	t.Parallel()

	unreadable := filepath.Join(t.TempDir(), "never-written.mkv")
	handler := pmsPreviewHandler(t, 1, unreadable)

	resp := postPreview(
		t,
		handler,
		"application/json",
		strings.NewReader(`{"mediaId":"7","duration":10}`),
	)

	assert.Equal(t, fiber.StatusBadRequest, resp.status)

	assert.Equal(t, api.MediaPathUnresolved, resp.failure.Error)
	assert.Contains(t, resp.failure.Message, "Outtake can't read this title's file",
		"the failure names the source rather than the request")
}

func TestPreviewRedirectsAFormPostWithAnUnreadableMark(t *testing.T) {
	t.Parallel()

	marks := map[string]url.Values{
		"an unparseable start": {
			"startTime": []string{"not-a-timecode"},
			"endTime":   []string{"00:00:10"},
		},
		"an unparseable end": {
			"startTime": []string{"00:00:00"},
			"endTime":   []string{"tomorrow"},
		},
		"a start of only spaces": {
			"startTime": []string{"   "},
			"endTime":   []string{"00:00:10"},
		},
	}

	for name, form := range marks {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler, _ := e2ePreviewHandler(t, 1)

			mediaID := sourceFile(t)
			form.Set("mediaId", mediaID)

			resp := postPreviewForm(t, handler, form)
			path, _ := resp.redirect(t)

			assert.Equal(t, "/media/item/"+mediaID, path,
				"the form post returns to the page it came from")
			assert.Contains(t, resp.flash, "as a time, such as 00:01:23.456")
		})
	}
}

func TestPreviewRedirectsAFormPostForAnUnresolvableSource(t *testing.T) {
	t.Parallel()

	handler := unselectedPreviewHandler(t, 1, "")

	resp := postPreviewForm(t, handler, previewWindowForm("9999", 0, 10))

	path, _ := resp.redirect(t)

	assert.Equal(t, "/media/item/9999", path, "the form post names the page it came from")
	assert.Equal(t, "Choose a Plex server under Servers first", resp.flash)
}

func TestPreviewRefusesWhenTheQueueIsFull(t *testing.T) {
	t.Parallel()

	handler := pmsPreviewHandler(t, 1, sourceFile(t))

	fillPreviewQueue(t, handler, 1)

	resp := postPreview(
		t,
		handler,
		"application/json",
		strings.NewReader(`{"mediaId":"7","duration":10}`),
	)

	assert.Equal(t, fiber.StatusTooManyRequests, resp.status)

	assert.Equal(t, api.PreviewBusy, resp.failure.Error)
	assert.Equal(t, "Too many previews are rendering. Try again shortly.", resp.failure.Message)
}

func TestPreviewRedirectsAFormPostRefusedByAFullQueue(t *testing.T) {
	t.Parallel()

	handler := pmsPreviewHandler(t, 1, sourceFile(t))

	fillPreviewQueue(t, handler, 1)

	resp := postPreviewForm(t, handler, previewWindowForm("7", 0, 10))

	path, _ := resp.redirect(t)

	assert.Equal(t, "/media/item/7", path, "the form post returns to the page it came from")
	assert.Equal(t, "Too many previews are rendering. Try again shortly.", resp.flash)
}

func TestPreviewFileReportsAnAbsentPublishedPreview(t *testing.T) {
	t.Parallel()

	handler, _ := previewTestHandler(t, 1)

	const id = "b3c1f0d2e4a5468798ab0c1d2e3f40516273849"

	assert.Equal(t,
		fiber.StatusNotFound,
		getPreviewFile(t, handler, id),
		"a safe id whose preview was never published is reported as absent",
	)
}

// TestPreviewRefusesASelectionAClipCouldNotHave covers the bounds a preview
// shares with the clip it previews: the configured cap, the source's end, the
// audio tracks it carries, and a start that is not negative.
func TestPreviewRefusesASelectionAClipCouldNotHave(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   func(source string) string
		reason string
	}{
		{
			name:   "a window longer than the configured cap",
			body:   func(source string) string { return previewBody(source, 0, 30, 0) },
			reason: "A clip can be at most 20s",
		},
		{
			name:   "a window past the end of the source",
			body:   func(source string) string { return previewBody(source, 115, 10, 0) },
			reason: "is past the end of the title",
		},
		{
			name:   "an audio track the source does not carry",
			body:   func(source string) string { return previewBody(source, 0, 10, 3) },
			reason: "This title has no audio track",
		},
		{
			name:   "a negative start",
			body:   func(source string) string { return previewBody(source, -5, 10, 0) },
			reason: "The start can't be negative",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, store := e2ePreviewHandler(t, 1)

			handler.cfg.MaxClipDur = 20 * time.Second

			resp := postPreview(
				t, handler, "application/json", strings.NewReader(test.body(sourceFile(t))),
			)

			assert.Equal(t, fiber.StatusBadRequest, resp.status)
			assert.Contains(t, resp.failure.Message, test.reason)
			assert.Empty(t, previewFiles(t, store), "nothing is rendered for a refused window")
		})
	}
}

// previewBody builds a JSON preview request.
//
// Parameters:
//   - source: The media id, which names a local file in the e2e environment.
//   - start: Start mark in seconds.
//   - length: Window length in seconds.
//   - audio: Audio track position.
//
// Returns:
//   - body: The JSON request body.
func previewBody(source string, start, length, audio int) string {
	return fmt.Sprintf(
		`{"mediaId":%q,"clipType":"clip","startTime":%d,"duration":%d,"audioIndex":%d}`,
		source, start, length, audio,
	)
}

// previewFiles lists the files in the preview directory.
//
// Parameters:
//   - t: The test that is looking.
//   - store: The storage previews are published into.
//
// Returns:
//   - names: The file names, empty when the directory holds none.
func previewFiles(t *testing.T, store *blob.Storage) []string {
	t.Helper()

	entries, err := os.ReadDir(store.PreviewsDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}
