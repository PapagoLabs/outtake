// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	librarymocks "github.com/PapagoLabs/outtake/internal/plex/library/mocks"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// sourceHandler builds a handler whose source resolver cannot reach Plex.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - duration: Length the stubbed prober reports.
//
// Returns:
//   - handler: The handler under test.
func sourceHandler(t *testing.T, duration time.Duration) *Handler {
	t.Helper()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return newSourceHandler(t, db, duration, &config.Config{
		MaxClipDur: 15 * time.Minute,
	})
}

// newSourceHandler builds a handler over an open database and one source config.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - db: Database the profile resolution reads through.
//   - duration: Length the stubbed prober reports.
//   - sourceCfg: Configuration the source resolver runs under.
//
// Returns:
//   - handler: The handler under test.
func newSourceHandler(
	t *testing.T,
	db *database.DB,
	duration time.Duration,
	sourceCfg *config.Config,
) *Handler {
	t.Helper()

	selected := librarymocks.NewMockServerSelection(t)
	selected.EXPECT().Client().Return(nil, plex.EmptyServer(), false).Maybe()

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	return &Handler{
		db:          db,
		cfg:         &config.Config{MaxClipDur: 15 * time.Minute},
		clipQueue:   jobQueue,
		clipPaths:   blob.NewPaths(t.TempDir()),
		clipStorage: &blob.Storage{},
		sources: library.NewMediaSource(
			sourceCfg, selected, &stubProber{duration: duration},
		),
	}
}

// localSourceFile writes a media file a test can name as its own source.
//
// Parameters:
//   - t: The test the file belongs to.
//
// Returns:
//   - path: The path of the written file.
func localSourceFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("media"), 0o600))

	return path
}

// createClip serves one create request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - body: JSON body to post.
//
// Returns:
//   - status: Response status code.
//   - body: Response body.
func createClip(t *testing.T, handler *Handler, body string) (int, string) {
	t.Helper()

	app := fiber.New()
	app.Post("/api/clips/create", handler.Create)

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(body),
	)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	answer, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(answer)
}

func TestResolveInputReportsASourceItCannotResolve(t *testing.T) {
	t.Parallel()

	handler := sourceHandler(t, 2*time.Hour)

	path, err := handler.resolveInput(t.Context(), "42")

	require.Error(t, err)
	assert.Empty(t, path)
	assert.Contains(t, err.Error(), "resolve input",
		"the failure names the step that could not be done")
}

func TestResolveNewClipReportsAProfileItCannotResolve(t *testing.T) {
	t.Parallel()

	handler := sourceHandler(t, 2*time.Hour)

	app := fiber.New()

	var (
		gotPath string
		gotCode api.ErrorCode
		gotErr  error
	)

	app.Post("/api/clips/create", func(ctx fiber.Ctx) error {
		gotPath, gotCode, gotErr = handler.resolveNewClip(
			ctx,
			&api.ClipRequest{
				MediaID:   "42",
				Quality:   "no-such-profile",
				ClipType:  string(clipdom.TypeClip),
				StartTime: 10,
				Duration:  15,
			},
			clipdom.TypeClip,
		)

		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(""),
	)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	require.Error(t, gotErr)
	assert.Equal(t, api.InvalidQuality, gotCode)
	assert.Empty(t, gotPath)
	assert.Contains(t, gotErr.Error(), "unknown clip profile",
		"the reason the quality was refused reaches the caller")
}

func TestResolveNewClipReportsASourceItCannotResolve(t *testing.T) {
	t.Parallel()

	handler := sourceHandler(t, 2*time.Hour)

	app := fiber.New()

	var (
		gotPath string
		gotCode api.ErrorCode
		gotErr  error
	)

	app.Post("/api/clips/create", func(ctx fiber.Ctx) error {
		req := &api.ClipRequest{
			MediaID:   "42",
			Quality:   "",
			ClipType:  string(clipdom.TypeClip),
			StartTime: 10,
			Duration:  15,
		}

		gotPath, gotCode, gotErr = handler.resolveNewClip(ctx, req, clipdom.TypeClip)

		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(""),
	)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	require.Error(t, gotErr)
	assert.Equal(t, api.MediaPathUnresolved, gotCode)
	assert.Empty(t, gotPath)
	assert.Contains(t, gotErr.Error(), "resolve input")
}

func TestCreateReportsAProfileItCannotResolve(t *testing.T) {
	t.Parallel()

	handler := sourceHandler(t, 2*time.Hour)

	status, body := createClip(t, handler,
		`{"mediaId":"42","mediaTitle":"Movie","clipType":"clip","quality":"no-such-profile",
		 "startTime":10,"duration":15}`)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Contains(t, body, api.InvalidQuality)
}

func TestCreateReportsASourceItCannotResolve(t *testing.T) {
	t.Parallel()

	handler := sourceHandler(t, 2*time.Hour)

	status, body := createClip(t, handler,
		`{"mediaId":"42","mediaTitle":"Movie","clipType":"clip","quality":"",
		 "startTime":10,"duration":15}`)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Contains(t, body, api.MediaPathUnresolved)
}

func TestValidateEditBoundsASelectionByTheSourceLength(t *testing.T) {
	t.Parallel()

	handler := sourceHandler(t, time.Minute)

	app := fiber.New()

	var gotErr error

	app.Post("/api/clips/create", func(ctx fiber.Ctx) error {
		gotErr = handler.validateEdit(ctx.Context(), "/media/movie.mkv", clipdom.Edit{
			Type:   clipdom.TypeClip,
			Start:  2 * time.Minute,
			Length: 30 * time.Second,
		})

		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(""),
	)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "validate range")
	assert.Contains(t, gotErr.Error(), "outside the media",
		"the source is only a minute long, so a later start cannot be cut")
}

func TestValidateEditAcceptsAnythingFromAnUnprobedSource(t *testing.T) {
	t.Parallel()

	handler := sourceHandler(t, 0)

	app := fiber.New()

	var gotErr error

	app.Post("/api/clips/create", func(ctx fiber.Ctx) error {
		gotErr = handler.validateEdit(ctx.Context(), "/media/movie.mkv", clipdom.Edit{
			Type:   clipdom.TypeClip,
			Start:  2 * time.Hour,
			Length: 10 * time.Minute,
		})

		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(""),
	)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.NoError(t, gotErr,
		"a source whose length could not be probed is treated as long enough, "+
			"so the clip is bounded by the configured maximum alone")
}

func TestResolveInputResolvesALocalSourcePath(t *testing.T) {
	t.Parallel()

	source := localSourceFile(t)

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	handler := newSourceHandler(t, db, 2*time.Hour,
		&config.Config{MaxClipDur: 15 * time.Minute, Env: "e2e"})

	path, err := handler.resolveInput(t.Context(), source)

	require.NoError(t, err)
	assert.Equal(t, source, path)
}

func TestResolveNewClipAcceptsAResolvableSource(t *testing.T) {
	t.Parallel()

	source := localSourceFile(t)

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	handler := newSourceHandler(t, db, 2*time.Hour,
		&config.Config{MaxClipDur: 15 * time.Minute, Env: "e2e"})

	app := fiber.New()

	var (
		gotPath string
		gotCode api.ErrorCode
		gotErr  error
	)

	app.Post("/api/clips/create", func(ctx fiber.Ctx) error {
		req := &api.ClipRequest{
			MediaID:   source,
			ClipType:  string(clipdom.TypeClip),
			StartTime: 10,
			Duration:  15,
		}

		gotPath, gotCode, gotErr = handler.resolveNewClip(ctx, req, clipdom.TypeClip)

		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(""),
	)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	require.NoError(t, gotErr)
	assert.Empty(t, gotCode, "a clip that resolved cleanly has nothing to report")
	assert.Equal(t, source, gotPath)
}

func TestResolveNewClipRestatesTheResolvedQuality(t *testing.T) {
	t.Parallel()

	source := localSourceFile(t)

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	handler := newSourceHandler(t, db, 2*time.Hour,
		&config.Config{MaxClipDur: 15 * time.Minute, Env: "e2e"})

	var resolved string

	app := fiber.New()
	app.Post("/api/clips/create", func(ctx fiber.Ctx) error {
		req := &api.ClipRequest{
			MediaID:   source,
			ClipType:  string(clipdom.TypeClip),
			StartTime: 10,
			Duration:  15,
		}

		_, _, resolveErr := handler.resolveNewClip(ctx, req, clipdom.TypeClip)
		if resolveErr != nil {
			return ctx.SendStatus(fiber.StatusBadRequest)
		}

		resolved = req.Quality

		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(""),
	)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, storedProfileID(t, db, "1080p"), resolved,
		"the request is rewritten with the profile the clip will actually use")
}

func TestValidateEditStillBoundsAnUnprobedSourceByTheMaximum(t *testing.T) {
	t.Parallel()

	handler := sourceHandler(t, 0)

	app := fiber.New()

	var gotErr error

	app.Post("/api/clips/create", func(ctx fiber.Ctx) error {
		gotErr = handler.validateEdit(ctx.Context(), "/media/movie.mkv", clipdom.Edit{
			Type:   clipdom.TypeClip,
			Length: 2 * time.Hour,
		})

		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(""),
	)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	require.Error(t, gotErr,
		"an unprobed source has no length, but the configured maximum still stands")
	assert.Contains(t, gotErr.Error(), "validate range")
}

// resolvableHandler builds a handler whose source resolves to a local file.
//
// Parameters:
//   - t: The test the handler belongs to.
//
// Returns:
//   - handler: The handler under test.
//   - source: Path of the media file a create names as its source.
func resolvableHandler(t *testing.T) (*Handler, string) {
	t.Helper()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	source := localSourceFile(t)

	return newSourceHandler(t, db, 2*time.Hour,
		&config.Config{MaxClipDur: 15 * time.Minute, Env: "e2e"}), source
}

// createJSON serves one JSON create request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - body: JSON body to post.
//   - htmx: Whether to mark the request as coming from HTMX.
//
// Returns:
//   - status: Response status code.
//   - body: Response body.
func createJSON(
	t *testing.T,
	handler *Handler,
	body string,
	htmx bool,
) (int, string) {
	t.Helper()

	app := fiber.New()
	app.Post("/api/clips/create", handler.Create)

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(body),
	)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	if htmx {
		req.Header.Set(routes.HeaderHXRequest, "true")
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	answer, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(answer)
}

// createJSONBody builds a create request body naming one source.
//
// Parameters:
//   - mediaID: Media the clip is cut from.
//   - quality: Profile id, empty for the default.
//
// Returns:
//   - body: The encoded JSON request.
func createJSONBody(mediaID, quality string) string {
	return `{"mediaId":"` + mediaID + `","mediaTitle":"Movie",` +
		`"clipType":"clip","quality":"` + quality +
		`","startTime":10,"duration":15}`
}

func TestCreateQueuesAClipItResolved(t *testing.T) {
	t.Parallel()

	handler, source := resolvableHandler(t)

	status, body := createJSON(t, handler, createJSONBody(source, ""), false)

	require.Equal(t, fiber.StatusCreated, status)
	assert.Contains(t, body, `"mediaId":"`+source+`"`)
	assert.Contains(t, body, `"status":"pending"`)

	stored, err := handler.db.ListClips(t.Context())
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, storedProfileID(t, handler.db, "1080p"), stored[0].Quality,
		"the empty quality asked for was resolved to the seeded default")
	assert.NotEmpty(
		t,
		stored[0].OutputPath,
		"the output path is assigned before the queue takes it",
	)
	assert.Equal(t, source, stored[0].InputPath)
}

func TestCreateSwapsARejectionIntoTheFlash(t *testing.T) {
	t.Parallel()

	handler, _ := resolvableHandler(t)

	status, body := createJSON(t, handler, createJSONBody("42", "no-such-profile"), true)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Contains(t, body, `hx-target="#flash"`,
		"the rejection is swapped into the flash slot the page already has")
	assert.Contains(t, body, "unknown clip profile",
		"the reason the clip was refused reaches the page")
	assert.NotContains(t, body, api.InvalidQuality,
		"the flash carries the reason, which is what the page has to read")
}

func TestCreateAnswersAnAPICallerWithTheErrorCode(t *testing.T) {
	t.Parallel()

	handler, _ := resolvableHandler(t)

	status, body := createJSON(t, handler,
		createJSONBody("42", "no-such-profile"), false)

	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Contains(t, body, api.InvalidQuality)
	assert.NotContains(t, body, "hx-target", "an API caller is given no markup")
}

func TestCreateRedirectsAFormPostBackToTheMediaItem(t *testing.T) {
	t.Parallel()

	handler, source := resolvableHandler(t)

	app := fiber.New()
	app.Post("/api/clips/create", handler.Create)

	form := url.Values{
		"mediaId":    {source},
		"mediaTitle": {"Movie"},
		"clipType":   {string(clipdom.TypeClip)},
		"startTime":  {"00:00:10.000"},
		"endTime":    {"00:00:25.000"},
		"quality":    {storedProfileID(t, handler.db, "1080p")},
	}

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/create", strings.NewReader(form.Encode()),
	)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	require.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/media/item/"+url.PathEscape(source),
		resp.Header.Get(fiber.HeaderLocation),
		"a form post lands back on the item the clip was cut from")
}
