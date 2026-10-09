// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

func TestBuildJobStampsAPendingRecord(t *testing.T) {
	t.Parallel()

	before := time.Now()

	job := buildJob(
		&api.ClipRequest{MediaID: "42", MediaTitle: "Movie", Quality: "archive"},
		clipdom.TypeGIF,
		"/media/movie.mkv",
		true,
	)

	assert.NotEmpty(t, job.ID, "every clip is stored under a fresh id")
	assert.Equal(t, clipdom.TypeGIF, job.Type)
	assert.Equal(t, clipdom.StatusPending, job.Status, "a new clip is queued, not rendered")
	assert.Zero(t, job.Progress)
	assert.Empty(t, job.Error)
	assert.Empty(t, job.OutputPath, "the output path is assigned once the queue takes the clip")
	assert.Equal(t, "/media/movie.mkv", job.InputPath)
	assert.Equal(t, "archive", job.Quality)
	assert.True(t, job.PreserveHDR)
	assert.False(t, job.CreatedAt.Before(before), "the record is stamped when it is built")
	assert.Equal(t, job.CreatedAt, job.UpdatedAt, "a record that has never changed is not newer")
}

func TestBuildJobNamesAnUnnamedClipFromItsSource(t *testing.T) {
	t.Parallel()

	named := buildJob(
		&api.ClipRequest{Name: "Intro", MediaTitle: "Movie"},
		clipdom.TypeClip,
		"/media/movie.mkv",
		false,
	)
	assert.Equal(t, "Intro", named.Name)

	unnamed := buildJob(
		&api.ClipRequest{MediaTitle: "Movie"},
		clipdom.TypeClip,
		"/media/movie.mkv",
		false,
	)
	assert.Equal(t, "Movie", unnamed.Name,
		"a clip with no name of its own is named after what it was cut from")
}

func TestBuildJobReadsTheMarksAsDurations(t *testing.T) {
	t.Parallel()

	job := buildJob(
		&api.ClipRequest{StartTime: 12.5, Duration: 5.25},
		clipdom.TypeClip,
		"/media/movie.mkv",
		false,
	)

	assert.Equal(t, 12500*time.Millisecond, job.StartTime)
	assert.Equal(t, 5250*time.Millisecond, job.Duration)
}

func TestCreateRejectsATypeItCannotProduce(t *testing.T) {
	t.Parallel()

	handler := &Handler{}

	app := fiber.New()
	app.Post("/api/clips", handler.Create)

	post := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/clips",
		strings.NewReader(`{"mediaId":"42","clipType":"hologram","startTime":10,"duration":15}`),
	)
	post.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(post)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, bodyText(t, resp), api.InvalidClipType,
		"the code names which field was wrong")
}

func bodyText(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body)
}

// TestKeepHDRFollowsTheProfile covers a clip's Keep HDR, which is its
// profile's setting: High HDR keeps HDR, and High, Medium, and the default
// profile convert.
func TestKeepHDRFollowsTheProfile(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/keep.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	handler := &Handler{db: db}

	tests := []struct {
		name    string
		quality string
		want    bool
	}{
		{name: "High HDR keeps HDR", quality: "high-hdr", want: true},
		{name: "High converts", quality: "high", want: false},
		{name: "Medium converts", quality: "medium", want: false},
		{name: "the default profile converts", quality: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, handler.keepHDR(t.Context(), test.quality))
		})
	}
}
