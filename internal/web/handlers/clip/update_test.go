// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

type updateAnswer struct {
	status int
	header http.Header
	body   string
}

func updateTestDB(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	job := testClipJob("stored", clipdom.TypeClip)

	job.StartTime = 30 * time.Second
	job.Duration = 20 * time.Second
	job.InputPath = "/media/movie.mkv"
	require.NoError(t, db.SaveClip(t.Context(), job))

	return db
}

func updateTestHandler(t *testing.T, db *database.DB) *Handler {
	t.Helper()

	return &Handler{
		db:          db,
		cfg:         &config.Config{MaxClipDur: 600 * time.Second},
		clipQueue:   queue.NewQueue(1, nil),
		clipStorage: &blob.Storage{},
		sources:     stubSources(t, 2*time.Hour),
	}
}

func updateMarks(start, end, profile string) url.Values {
	form := url.Values{
		"startTime": {start},
		"endTime":   {end},
	}

	if profile != "" {
		form.Set(routes.QueryQuality, profile)
	}

	return form
}

func htmxFormRequest(t *testing.T, id string, form url.Values) *http.Request {
	t.Helper()

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/clips/"+id+"/update",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	req.Header.Set("Hx-Request", "true")

	return req
}

func apiJSONRequest(t *testing.T, body string) *http.Request {
	t.Helper()

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/clips/stored/update",
		strings.NewReader(body),
	)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	return req
}

func postUpdate(t *testing.T, handler *Handler, req *http.Request) updateAnswer {
	t.Helper()

	app := fiber.New()
	app.Post("/api/clips/:id/update", handler.Update)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return updateAnswer{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func TestUpdateSwapsARejectionIntoTheFlashSlot(t *testing.T) {
	t.Parallel()

	staleType := updateMarks("00:00:30.000", "00:00:50.000", string(clipdom.ClipQualityMedium))
	staleType.Set("clipType", "hologram")

	tests := []struct {
		name   string
		form   url.Values
		reason string
	}{
		{
			name:   "a profile id that names no profile",
			form:   updateMarks("00:00:30.000", "00:00:50.000", "no-such-profile"),
			reason: "unknown clip profile",
		},
		{
			name:   "a selection past the end of the source",
			form:   updateMarks("11:00:00.000", "11:00:20.000", string(clipdom.ClipQualityMedium)),
			reason: "outside the media",
		},
		{
			name:   "a clip type the job cannot become",
			form:   staleType,
			reason: "clip type must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := updateTestDB(t)
			handler := updateTestHandler(t, db)

			answer := postUpdate(t, handler, htmxFormRequest(t, "stored", tt.form))

			assert.Equal(t, fiber.StatusOK, answer.status,
				"a rejection the page has to show cannot carry an error status")
			assert.Equal(t, routes.HXTargetNone, answer.header.Get(routes.HeaderHXReswap),
				"htmx is told to leave the card alone")
			assert.Contains(t, answer.body, `hx-swap-oob="true"`,
				"the banner is swapped out of band")
			assert.Contains(t, answer.body, `id="flash"`, "the banner lands in the flash slot")
			assert.Contains(t, answer.body, tt.reason, "the reason reaches the page")
			assert.NotContains(t, answer.body, "startTime",
				"the card is not re-rendered, so what was typed into it survives")

			stored, err := db.GetClip(t.Context(), "stored")
			require.NoError(t, err)
			assert.InDelta(t, 30, stored.StartTime.Seconds(), 0.0005,
				"the stored start is untouched")
			assert.InDelta(t, 20, stored.Duration.Seconds(), 0.0005,
				"the stored length is untouched")
		})
	}
}

func TestUpdateSwapsAMissingClipIntoTheFlashSlot(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	form := updateMarks("00:00:30.000", "00:00:50.000", string(clipdom.ClipQualityMedium))
	answer := postUpdate(t, handler, htmxFormRequest(t, "absent", form))

	assert.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, `hx-swap-oob="true"`)
	assert.Contains(t, answer.body, respond.NotFoundMessage, "the reason reaches the page")

	_, err := db.GetClip(t.Context(), "absent")
	require.ErrorIs(t, err, database.ErrClipNotFound,
		"a clip that was not there is still not there")
}

func TestUpdateKeepsJSONForAnAPICaller(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	answer := postUpdate(t, handler, apiJSONRequest(
		t, `{"clipType":"clip","quality":"no-such-profile"}`,
	))

	assert.Equal(t, fiber.StatusBadRequest, answer.status, "the status is unchanged")
	assert.JSONEq(t,
		`{"error":"invalid_quality","message":"apply quality: unknown clip profile"}`,
		answer.body, "the JSON body is unchanged")
	assert.Empty(t, answer.header.Get(routes.HeaderHXReswap),
		"an API caller is given no swap instructions")
	assert.NotContains(t, answer.body, "hx-swap-oob", "an API caller is given no markup")
}

func TestUpdateDoesNotHijackAnHTMXMarkedAPICaller(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	req := apiJSONRequest(t, `{"clipType":"clip","quality":"no-such-profile"}`)
	req.Header.Set("Hx-Request", "true")

	answer := postUpdate(t, handler, req)

	assert.Equal(t, fiber.StatusBadRequest, answer.status,
		"the status is the one the API contract gives, not the browser's 200")
	assert.Empty(t, answer.header.Get(routes.HeaderHXReswap),
		"an API caller is given no swap instructions")
	assert.NotContains(t, answer.body, "hx-swap-oob", "an API caller is given no markup")
}

func TestUpdateRejectsAnOutOfRangeSelectionForAnAPIClient(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	answer := postUpdate(t, handler, apiJSONRequest(t,
		`{"clipType":"clip","startTime":39600,"duration":20}`,
	))

	assert.Equal(t, fiber.StatusBadRequest, answer.status,
		"a selection past the end of the source cannot be stored")
	assert.Empty(t, answer.header.Get(routes.HeaderHXReswap),
		"an API caller is given no swap instructions")
	assert.NotContains(t, answer.body, "hx-swap-oob", "an API caller is given no markup")

	stored, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)
	assert.InDelta(t, 30, stored.StartTime.Seconds(), 0.0005, "the stored start is untouched")
	assert.InDelta(t, 20, stored.Duration.Seconds(), 0.0005, "the stored length is untouched")
}

func TestUpdateKeepsAMalformedMarkOutOfTheRow(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	form := updateMarks("00:00:50.000", "00:01:10.000", string(clipdom.ClipQualityMedium))
	form.Set("startTime", "not-a-mark")

	answer := postUpdate(t, handler, htmxFormRequest(t, "stored", form))

	assert.Equal(t, fiber.StatusOK, answer.status,
		"a rejection the page has to show cannot carry an error status")
	assert.Contains(t, answer.body, "timecode", "the reason names what the field wanted")
	assert.NotContains(t, answer.body, "must be between", "a bad mark is not a range error")

	stored, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)
	assert.InDelta(t, 30, stored.StartTime.Seconds(), 0.0005,
		"a mark the server cannot read leaves the stored window alone")
}

func TestUpdateKeepsARejectedSelectionOutOfTheRow(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	answer := postUpdate(t, handler, apiJSONRequest(t,
		`{"clipType":"clip","startTime":39600,"duration":20}`,
	))

	assert.Equal(t, fiber.StatusBadRequest, answer.status)

	stored, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)
	assert.InDelta(t, 30, stored.StartTime.Seconds(), 0.0005, "the stored start is untouched")
	assert.InDelta(t, 20, stored.Duration.Seconds(), 0.0005, "the stored length is untouched")
}

func TestUpdateSwapsTheCardBackInAfterAnAcceptedEdit(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	form := updateMarks("00:00:42.345", "00:00:50.345", string(clipdom.ClipQualityMedium))
	form.Set("name", "Outro")
	form.Set("audioIndex", "2")
	form.Set("cropBlackBars", routes.FormChecked)

	answer := postUpdate(t, handler, htmxFormRequest(t, "stored", form))

	require.Equal(t, fiber.StatusOK, answer.status,
		"an accepted edit is answered with the card to swap in")
	assert.Contains(t, answer.body, "Outro", "the card carries what was just saved")
	assert.Empty(t, answer.header.Get(routes.HeaderHXReswap),
		"an accepted edit needs no reswap, the card is the whole answer")

	stored, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)
	assert.Equal(t, "Outro", stored.Name)
	assert.InDelta(t, 42.345, stored.StartTime.Seconds(), 0.0005)
	assert.InDelta(t, 8, stored.Duration.Seconds(), 0.0005)
	assert.Equal(t, 2, stored.AudioIndex)
	assert.True(t, stored.CropBlackBars)
	assert.Equal(t, string(clipdom.ClipQualityMedium), stored.Quality)
}

func TestQueueRegenerateAfterACancel(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	stored, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)

	stored.Status = clipdom.StatusCancelled
	stored.OutputPath = ""

	require.NoError(t, handler.queueRegenerate(stored))

	restored := handler.clipQueue.GetJob("stored")
	require.NotNil(t, restored,
		"a finished render has to come back into the queue or the flag does nothing")
	assert.Equal(t, clipdom.StatusPending, restored.Status,
		"it comes back queued, not completed")
	assert.NotEmpty(t, restored.OutputPath, "the output path is rebuilt for the type")
}
