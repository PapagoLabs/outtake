// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package respond

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

func newTestApp(config ...fiber.Config) *fiber.App {
	if len(config) == 0 {
		return fiber.New()
	}

	return fiber.New(config[0])
}

// testFilePath writes a small file and returns its path.
//
// Parameters:
//   - t: The test the file belongs to.
//
// Returns:
//   - path: The written file.
func testFilePath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(path, []byte("encoded clip"), 0o600))

	return path
}

func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()

	require.NoError(t, resp.Body.Close())
}

func bodyText(t *testing.T, resp *http.Response) string {
	t.Helper()

	contents, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(contents)
}

// formRequest builds a urlencoded form post, optionally marked as an htmx
// request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - target: Request target.
//   - isHTMX: Whether to mark the request as issued by htmx.
//
// Returns:
//   - req: The request to hand to app.Test.
func formRequest(t *testing.T, target string, isHTMX bool) *http.Request {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, nil)
	req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")

	if isHTMX {
		req.Header.Set(routes.HeaderHXRequest, hxRequestValue)
	}

	return req
}

// writeBody writes markup into the response body.
//
// Parameters:
//   - writer: The response body writer.
//   - markup: The markup to write.
//
// Returns:
//   - err: Non-nil when the write fails.
func writeBody(writer io.Writer, markup string) error {
	_, err := writer.Write([]byte(markup))
	if err != nil {
		return fmt.Errorf("write body: %w", err)
	}

	return nil
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Get("/x", func(ctx fiber.Ctx) error {
		return WriteJSON(ctx, fiber.StatusCreated, fiber.Map{"ok": true})
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/x", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
	assert.Contains(t, bodyText(t, resp), `"ok":true`)
}

func TestWriteErrorJSONForAnAPIRequest(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Get("/x", func(ctx fiber.Ctx) error {
		return WriteError(ctx, fiber.StatusBadRequest, api.InvalidRequest, "bad duration")
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/x", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	body := bodyText(t, resp)

	assert.Contains(t, body, api.InvalidRequest)
	assert.Contains(t, body, "bad duration")
}

func TestWriteErrorHTMXRedirectsToAFlashBanner(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Get("/x", func(ctx fiber.Ctx) error {
		return WriteError(ctx, fiber.StatusBadRequest, api.InvalidRequest, "bad duration")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.Header.Set(routes.HeaderHXRequest, "true")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode,
		"a swap cannot follow a redirect, so the status still carries the failure")
	assert.Contains(t, bodyText(t, resp), "bad duration")
}

func TestWriteErrorFormRedirectsToTheReferer(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Post("/clips", func(ctx fiber.Ctx) error {
		return WriteError(ctx, fiber.StatusBadRequest, api.InvalidRequest, "bad duration")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/clips", nil)
	req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")
	req.Header.Set(fiber.HeaderReferer, "http://localhost:8080/media/item/42?start=30")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t,
		"/media/item/42?error=bad+duration&start=30",
		resp.Header.Get(fiber.HeaderLocation),
		"a form post goes back to the page it came from with the reason in the query")
}

func TestWriteErrorFormRedirectsToTheMediaItem(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Post("/clips", func(ctx fiber.Ctx) error {
		return WriteError(ctx, fiber.StatusBadRequest, api.InvalidRequest, "bad duration")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/clips", nil)
	req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t,
		"/?error=bad+duration",
		resp.Header.Get(fiber.HeaderLocation),
		"a post naming no media and no referer lands on the dashboard with the reason")
}

func TestWriteNotFound(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Get("/x", func(ctx fiber.Ctx) error {
		return WriteNotFound(ctx)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/x", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assert.Contains(t, bodyText(t, resp), api.NotFound)
}

func TestRedirectTo(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Get("/x", func(ctx fiber.Ctx) error {
		return RedirectTo(ctx, routes.PathClips)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/x", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode, "Fiber redirects a GET with 303")
	assert.Equal(t, routes.PathClips, resp.Header.Get(fiber.HeaderLocation))
}

func TestSendTextAndStatusCode(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Get("/text", func(ctx fiber.Ctx) error {
		return SendText(ctx, "pong")
	})
	app.Get("/status", func(ctx fiber.Ctx) error {
		return SendStatusCode(ctx, fiber.StatusNoContent)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/text", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, "pong", bodyText(t, resp))

	resp, err = app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/status", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
}

func TestRenderHTMLSetsTheContentType(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Get("/x", func(ctx fiber.Ctx) error {
		return RenderHTML(ctx, func(writer io.Writer) error {
			return writeBody(writer, "<h1>hi</h1>")
		})
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/x", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, contentTypeHTML, resp.Header.Get(fiber.HeaderContentType))
	assert.Contains(t, bodyText(t, resp), "<h1>hi</h1>")
}

func TestSendRangedFile(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	app.Get("/x", func(ctx fiber.Ctx) error {
		return SendRangedFile(ctx, testFilePath(t))
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/x", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Contains(t, bodyText(t, resp), "clip")
}

func TestIsHTMXRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		isHTMX bool
		want   bool
	}{
		{name: "a plain request is not htmx", want: false},
		{name: "a marked request is htmx", isHTMX: true, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got bool

			app := newTestApp()
			app.Get("/x", func(ctx fiber.Ctx) error {
				got = IsHTMXRequest(ctx)

				return nil
			})

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)

			if test.isHTMX {
				req.Header.Set(routes.HeaderHXRequest, hxRequestValue)
			}

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer closeBody(t, resp)

			assert.Equal(t, test.want, got)
		})
	}
}

func TestIsFormRequest(t *testing.T) {
	t.Parallel()

	var got bool

	app := newTestApp()
	app.Post("/x", func(ctx fiber.Ctx) error {
		got = IsFormRequest(ctx)

		return nil
	})

	resp, err := app.Test(formRequest(t, "/x", false))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.True(t, got)
}

func TestHXTargetID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give string
		want string
	}{
		{give: "", want: ""},
		{give: "clip-list", want: "clip-list"},
		{give: "div#clip-list", want: "clip-list"},
		{give: "#clip-list", want: "clip-list"},
		{give: "main#main-content", want: "main-content"},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, HXTargetID(test.give))
	}
}

func TestPathWithError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		give    string
		message string
		want    string
	}{
		{
			name:    "a page path gains the error",
			give:    "/media",
			message: "bad duration",
			want:    "/media?error=bad+duration",
		},
		{
			name:    "existing query values are kept",
			give:    "/media?library=2",
			message: "bad duration",
			want:    "/media?error=bad+duration&library=2",
		},
		{
			name:    "an unparseable location falls back to the dashboard",
			give:    "://bad",
			message: "bad duration",
			want:    "/?error=bad+duration",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, PathWithError(test.give, test.message))
		})
	}
}

func TestRefererPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want string
	}{
		{
			name: "a path and query are kept",
			give: "http://localhost:8080/media/item/42?start=30",
			want: "/media/item/42?start=30",
		},
		{
			name: "a bare path is kept",
			give: "/media",
			want: "/media",
		},
		{
			name: "an absolute URL without a path falls back to the dashboard",
			give: "http://localhost:8080",
			want: routes.PathRoot,
		},
		{
			name: "an unparseable referer falls back to the dashboard",
			give: "://bad",
			want: routes.PathRoot,
		},
		{
			name: "an empty referer falls back to the dashboard",
			give: "",
			want: routes.PathRoot,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, RefererPath(test.give))
		})
	}
}

func TestRefererOrFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		referer  string
		fallback string
		want     string
	}{
		{
			name:     "a referer wins over the fallback",
			referer:  "http://localhost:8080/clips",
			fallback: routes.PathRoot,
			want:     routes.PathClips,
		},
		{
			name:     "the fallback is used when there is no referer",
			fallback: routes.PathMedia,
			want:     routes.PathMedia,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got string

			app := newTestApp()
			app.Get("/x", func(ctx fiber.Ctx) error {
				got = RefererOrFallback(ctx, test.fallback)

				return nil
			})

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)

			if test.referer != "" {
				req.Header.Set(fiber.HeaderReferer, test.referer)
			}

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer closeBody(t, resp)

			assert.Equal(t, test.want, got)
		})
	}
}

func TestFormErrorLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		referer  string
		form     string
		wantPath string
	}{
		{
			name:     "a referer wins",
			referer:  "http://localhost:8080/clips?type=gif",
			wantPath: "/clips",
		},
		{
			name:     "a posted media id is the next best thing",
			form:     "mediaId=42",
			wantPath: "/media/item/42",
		},
		{
			name:     "the dashboard is the last resort",
			wantPath: routes.PathRoot,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got string

			app := newTestApp()
			app.Post("/x", func(ctx fiber.Ctx) error {
				got = FormErrorLocation(ctx, "bad duration")

				return nil
			})

			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"/x",
				strings.NewReader(test.form),
			)
			req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")

			if test.referer != "" {
				req.Header.Set(fiber.HeaderReferer, test.referer)
			}

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer closeBody(t, resp)

			assert.Equal(t, test.wantPath, got[:len(test.wantPath)])
			assert.Contains(t, got, "error=bad+duration")
		})
	}
}

func TestTargetsElement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "the bare element id", target: "media-more", want: true},
		{name: "the id behind a selector", target: "div#media-more", want: true},
		{name: "another element", target: "media-prev", want: false},
		{name: "no target at all", target: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got bool

			app := newTestApp()
			app.Get("/x", func(ctx fiber.Ctx) error {
				got = TargetsElement(ctx, "media-more")

				return nil
			})

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
			if test.target != "" {
				req.Header.Set(routes.HeaderHXTarget, test.target)
			}

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer closeBody(t, resp)

			assert.Equal(t, test.want, got)
		})
	}
}

func TestFormAndQueryIntegers(t *testing.T) {
	t.Parallel()

	var (
		formValue  int
		queryValue int
	)

	app := newTestApp()
	app.Post("/x", func(ctx fiber.Ctx) error {
		formValue = FormInt(ctx, "width")
		queryValue = QueryInt(ctx, "fps")

		return nil
	})

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/x?fps=24",
		strings.NewReader("width=nope"),
	)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Zero(t, formValue, "a value that is not a number reads as unset")
	assert.Equal(t, 24, queryValue)
}

func TestQueryValue(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "5", QueryValue("/media?library=5&start=10", "library"))
	assert.Equal(t, "10", QueryValue("/media?library=5&start=10", "start"))
	assert.Empty(t, QueryValue("/media?library=5", "parent"))
	assert.Empty(t, QueryValue("/media", "library"))
	assert.Empty(t, QueryValue("://bad url", "library"))
}

func TestPageErrorRendersJSONForTheAPI(t *testing.T) {
	t.Parallel()

	app := newTestApp(fiber.Config{ErrorHandler: PageError})
	app.Get("/api/clips/x", func(fiber.Ctx) error {
		return fiber.ErrNotFound
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/clips/x", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assert.Contains(t, bodyText(t, resp), "http_error")
}

func TestPageErrorRendersAPageForABrowser(t *testing.T) {
	t.Parallel()

	app := newTestApp(fiber.Config{ErrorHandler: PageError})
	app.Get("/x", func(fiber.Ctx) error {
		return fiber.ErrNotFound
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/x", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assert.Equal(t, contentTypeHTML, resp.Header.Get(fiber.HeaderContentType))
	assert.Contains(t, bodyText(t, resp), "That page does not exist.")
}

func TestPageErrorAnswersAnHTMXFlash(t *testing.T) {
	t.Parallel()

	app := newTestApp(fiber.Config{ErrorHandler: PageError})
	app.Get("/x", func(fiber.Ctx) error {
		return fiber.ErrNotFound
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.Header.Set(routes.HeaderHXRequest, hxRequestValue)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assert.Contains(t, bodyText(t, resp), "That page does not exist.",
		"a swap takes the banner rather than the whole page")
}

func TestPageErrorHidesAnInternalDetail(t *testing.T) {
	t.Parallel()

	app := newTestApp(fiber.Config{ErrorHandler: PageError})
	app.Get("/api/x", func(fiber.Ctx) error {
		return assert.AnError
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/x", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	assert.NotContains(t, bodyText(t, resp), assert.AnError.Error(),
		"an internal failure is not reported to the caller verbatim")
}
