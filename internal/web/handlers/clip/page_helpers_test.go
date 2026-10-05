// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

type pageAnswer struct {
	status int
	header http.Header
	body   string
}

func queueForTest(t *testing.T, jobs ...*clipdom.Job) *queue.Queue {
	t.Helper()

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	for _, job := range jobs {
		jobQueue.Restore(job)
	}

	return jobQueue
}

func seedRenderedClip(t *testing.T, path string) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("rendered"), 0o600))

	return path
}

func clipPageHandler(t *testing.T) (*Handler, *database.DB) {
	t.Helper()

	db, err := database.New(t.TempDir() + "/pages.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return New(
		queueForTest(t),
		nil,
		blob.Paths{},
		db,
		&config.Config{MaxClipDur: 15 * time.Minute},
		nil,
	), db
}

func serveMethod(
	t *testing.T,
	app *fiber.App,
	method, target string,
	htmx bool,
	hxTarget string,
	form string,
) pageAnswer {
	t.Helper()

	var req *http.Request

	if form == "" {
		req = httptest.NewRequestWithContext(t.Context(), method, target, nil)
	} else {
		req = httptest.NewRequestWithContext(
			t.Context(), method, target, strings.NewReader(form),
		)
		req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")
	}

	if htmx {
		req.Header.Set(routes.HeaderHXRequest, "true")
	}

	if hxTarget != "" {
		req.Header.Set(routes.HeaderHXTarget, hxTarget)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return pageAnswer{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func assertBodyContains(t *testing.T, body, want, reason string) {
	t.Helper()

	assert.Contains(t, body, want, "%s", reason)
}

func assertBodyOmits(t *testing.T, body, unwanted, reason string) {
	t.Helper()

	assert.NotContains(t, body, unwanted, "%s (found %q)", reason, unwanted)
}
