// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"io"
	"maps"
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

	job.Quality = storedProfileID(t, db, "1080p")
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

	staleType := updateMarks("00:00:30.000", "00:00:50.000", "")
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
			form:   updateMarks("11:00:00.000", "11:00:20.000", ""),
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

	form := updateMarks("00:00:30.000", "00:00:50.000", storedProfileID(t, db, "1080p"))
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

	form := updateMarks("00:00:50.000", "00:01:10.000", storedProfileID(t, db, "1080p"))
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

	form := updateMarks("00:00:42.345", "00:00:50.345", storedProfileID(t, db, "1080p"))
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
	assert.Equal(t, storedProfileID(t, db, "1080p"), stored.Quality)
}

// TestUpdateRegeneratesACanceledClip covers the regenerate flag: a settled
// clip is queued again with an output path for its type.
func TestUpdateRegeneratesACanceledClip(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	stored, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)

	stored.Status = clipdom.StatusCancelled
	stored.OutputPath = ""
	require.NoError(t, db.SaveClip(t.Context(), stored))

	form := updateMarks("00:00:30.000", "00:00:50.000", storedProfileID(t, db, "1080p"))
	form.Set("regenerate", "1")

	answer := postUpdate(t, handler, htmxFormRequest(t, "stored", form))
	require.Equal(t, fiber.StatusOK, answer.status)

	restored := handler.clipQueue.GetJob("stored")
	require.NotNil(t, restored,
		"a finished render has to come back into the queue or the flag does nothing")
	assert.Equal(t, clipdom.StatusPending, restored.Status,
		"it comes back queued, not completed")
	assert.NotEmpty(t, restored.OutputPath, "the output path is rebuilt for the type")
}

// TestUpdateOfAPartialJSONBodyKeepsEveryOtherField covers a JSON caller that
// sends only the field it changes.
func TestUpdateOfAPartialJSONBodyKeepsEveryOtherField(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)

	stored, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)

	stored.AudioIndex = 2
	stored.CropBlackBars = true
	stored.PreserveHDR = true
	require.NoError(t, db.SaveClip(t.Context(), stored))

	handler := updateTestHandler(t, db)

	answer := postUpdate(t, handler, apiJSONRequest(t, `{"name":"Renamed"}`))
	require.Equal(t, fiber.StatusSeeOther, answer.status, "an API caller is sent back to the clip")

	saved, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)

	assert.Equal(t, "Renamed", saved.Name)
	assert.Equal(t, clipdom.TypeClip, saved.Type)
	assert.InDelta(t, 30, saved.StartTime.Seconds(), 0.0005)
	assert.InDelta(t, 20, saved.Duration.Seconds(), 0.0005)
	assert.Equal(t, storedProfileID(t, db, "1080p"), saved.Quality)
	assert.Equal(t, 2, saved.AudioIndex)
	assert.True(t, saved.CropBlackBars)
	assert.True(t, saved.PreserveHDR)
}

// TestUpdateIsReadBackAtOnce covers a clip the queue holds: the list and the
// status route show the edit without a restart.
func TestUpdateIsReadBackAtOnce(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	stored, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)

	handler.clipQueue.Restore(stored)

	answer := postUpdate(t, handler, apiJSONRequest(t, `{"name":"Renamed"}`))
	require.Equal(t, fiber.StatusSeeOther, answer.status)

	assert.Equal(t, "Renamed", handler.lookupJob(t.Context(), "stored").Name,
		"a lookup reads the queue first, and the queue carries the edit")

	listed := handler.listJobs(t.Context())
	require.Len(t, listed, 1)
	assert.Equal(t, "Renamed", listed[0].Name)
}

// TestUpdateOfARenderingClipOnlyAcceptsARename covers the edits a running
// render can take without its file going stale.
func TestUpdateOfARenderingClipOnlyAcceptsARename(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)

	started := make(chan struct{})
	release := make(chan struct{})

	work := queue.NewQueue(1, func(context.Context, *clipdom.Job) error {
		close(started)
		<-release

		return nil
	})
	work.Start(t.Context())
	t.Cleanup(work.Stop)
	t.Cleanup(func() { close(release) })

	stored, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)
	require.NoError(t, work.Submit(stored))

	<-started

	handler := updateTestHandler(t, db)

	handler.clipQueue = work

	answer := postUpdate(t, handler, apiJSONRequest(t, `{"startTime":40}`))
	assert.Equal(t, fiber.StatusConflict, answer.status, "a new window would make the file stale")
	assert.InDelta(t, 30, work.GetJob("stored").StartTime.Seconds(), 0.0005,
		"and the refused edit changes nothing")

	answer = postUpdate(t, handler, apiJSONRequest(t, `{"name":"Renamed"}`))
	assert.Equal(t, fiber.StatusSeeOther, answer.status, "a rename leaves the file as it is")
	assert.Equal(t, "Renamed", work.GetJob("stored").Name)
}

// TestUpdateRefusesANegativeMark covers a JSON edit whose start or length is
// below zero, which a conversion to a duration would otherwise read as zero.
func TestUpdateRefusesANegativeMark(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`{"startTime":-5}`, `{"duration":-1}`} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()

			db := updateTestDB(t)
			handler := updateTestHandler(t, db)

			answer := postUpdate(t, handler, apiJSONRequest(t, body))
			assert.Equal(t, fiber.StatusBadRequest, answer.status)
			assert.Contains(t, answer.body, "must not be negative")

			stored, err := db.GetClip(t.Context(), "stored")
			require.NoError(t, err)
			assert.InDelta(
				t,
				30,
				stored.StartTime.Seconds(),
				0.0005,
				"the stored start is untouched",
			)
		})
	}
}

// TestUpdateSwapsInACardThatKeepsItsSourceChoices covers the card an edit
// swaps in: it keeps the audio track select the page showed, rather than
// dropping it until a reload, and offers no HDR choice for an HDR source.
func TestUpdateSwapsInACardThatKeepsItsSourceChoices(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	handler.sources = hdrSources(t)

	form := updateMarks("00:00:30.000", "00:00:50.000", storedProfileID(t, db, "1080p"))

	answer := postUpdate(t, handler, htmxFormRequest(t, "stored", form))

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.NotContains(t, answer.body, `name="preserveHdr"`, "Keep HDR is the profile's")
	assert.Contains(t, answer.body, `id="audio-stored"`, "the audio track select is still offered")
}

// TestUpdateTakesKeepHDRFromTheProfileOnlyWhenItRenders covers a stored
// value that no longer matches its profile: an edit that renders the clip
// again takes the profile's setting, and any other edit keeps the stored
// one, so it always describes the clip's file.
func TestUpdateTakesKeepHDRFromTheProfileOnlyWhenItRenders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  clipdom.Status
		profile string
		stored  string
		form    url.Values
		want    bool
	}{
		{
			name:    "a finished clip saved without rendering keeps its file's setting",
			status:  clipdom.StatusCompleted,
			profile: "1080p",
			want:    true,
		},
		{
			name:    "a regenerate takes the profile's",
			status:  clipdom.StatusCompleted,
			profile: "1080p",
			form:    url.Values{"regenerate": {routes.FormChecked}},
			want:    false,
		},
		{
			name:    "a regenerate under 4K HDR keeps HDR",
			status:  clipdom.StatusCompleted,
			profile: "4K HDR",
			form:    url.Values{"regenerate": {routes.FormChecked}},
			want:    true,
		},
		{
			name:    "a type change renders, so it takes the profile's",
			status:  clipdom.StatusCompleted,
			profile: "1080p",
			form:    url.Values{"clipType": {string(clipdom.TypeGIF)}},
			want:    false,
		},
		{
			name:    "a regenerate under a deleted profile keeps the stored setting",
			status:  clipdom.StatusCompleted,
			profile: "",
			stored:  "deleted-profile",
			form:    url.Values{"regenerate": {routes.FormChecked}},
			want:    true,
		},
		{
			name:    "a clip still waiting renders the edit, so it takes the profile's",
			status:  clipdom.StatusPending,
			profile: "1080p",
			want:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			db := updateTestDB(t)

			stored, err := db.GetClip(t.Context(), "stored")
			require.NoError(t, err)

			stored.PreserveHDR = true
			stored.Status = test.status

			if test.stored != "" {
				stored.Quality = test.stored
			}

			require.NoError(t, db.SaveClip(t.Context(), stored))

			handler := updateTestHandler(t, db)

			quality := ""
			if test.profile != "" {
				quality = storedProfileID(t, db, test.profile)
			}

			form := updateMarks("00:00:30.000", "00:00:50.000", quality)
			maps.Copy(form, test.form)

			answer := postUpdate(t, handler, htmxFormRequest(t, "stored", form))
			require.Equal(t, fiber.StatusOK, answer.status, answer.body)

			saved, err := db.GetClip(t.Context(), "stored")
			require.NoError(t, err)
			assert.Equal(t, test.want, saved.PreserveHDR)
		})
	}
}

// TestUpdateRefusesAKeepHDRChoiceFromAnAPICaller covers a JSON caller that
// still chooses HDR: the edit is refused with a message naming the profile
// setting, and the clip is unchanged.
func TestUpdateRefusesAKeepHDRChoiceFromAnAPICaller(t *testing.T) {
	t.Parallel()

	db := updateTestDB(t)
	handler := updateTestHandler(t, db)

	answer := postUpdate(t, handler, apiJSONRequest(t, `{"name":"Renamed","webSafeColor":true}`))

	assert.Equal(t, fiber.StatusBadRequest, answer.status)
	assert.Contains(t, answer.body, "clip profile setting")

	saved, err := db.GetClip(t.Context(), "stored")
	require.NoError(t, err)
	assert.Equal(t, "Intro", saved.Name)
}
