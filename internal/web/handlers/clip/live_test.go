// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	librarymocks "github.com/PapagoLabs/outtake/internal/plex/library/mocks"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	storagemocks "github.com/PapagoLabs/outtake/internal/store/blob/mocks"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/middleware/middlewaretest"
)

// fakeClips stands in for the catalog and the card renderer behind a feed.
type fakeClips struct {
	// jobs are the clips the catalog holds.
	jobs map[string]*clipdom.Job
	// failing are the clips whose regions fail to render.
	failing map[string]bool
}

// errRenderFailed stands in for a region that cannot be rendered.
var errRenderFailed = errors.New("render failed")

// lookup finds a clip, as the catalog does.
//
// Parameters:
//   - id: The clip.
//
// Returns:
//   - job: A copy of the clip, nil when it is gone.
func (clips *fakeClips) lookup(_ context.Context, id string) *clipdom.Job {
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
	if clips.failing[job.ID] {
		return "", errRenderFailed
	}

	return string(job.Status) + " " + strings.Repeat("|", job.Progress), nil
}

// updatedIDs returns the clips a feed's payloads update, in order.
//
// Parameters:
//   - t: The test reading the payloads.
//   - payloads: What one check reported.
//
// Returns:
//   - ids: The id of each update.
func updatedIDs(t *testing.T, payloads []string) []string {
	t.Helper()

	ids := make([]string, 0, len(payloads))

	for _, payload := range payloads {
		var update liveUpdate

		require.NoError(t, json.Unmarshal([]byte(payload), &update))

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

// TestClipFeedReportsEachChange covers what the feed reports on each check.
//
// The first check reports every clip it can find. Later checks report only a
// clip whose region changed. A region that fails to render is tried again on
// the next check.
func TestClipFeedReportsEachChange(t *testing.T) {
	t.Parallel()

	clips := &fakeClips{
		jobs:    map[string]*clipdom.Job{"a": rendering("a"), "b": rendering("b")},
		failing: map[string]bool{},
	}

	feed := &clipFeed{
		lookup: clips.lookup,
		render: clips.render,
		sent:   make(map[string]string),
		ids:    []string{"a", "b", "gone"},
	}

	assert.Equal(t, []string{"a", "b"}, updatedIDs(t, feed.check(t.Context())),
		"the first check reports every clip it finds")
	assert.Empty(t, feed.check(t.Context()), "nothing changed")

	clips.jobs["b"].Progress = 10
	clips.failing["b"] = true

	assert.Empty(t, feed.check(t.Context()), "a region that fails to render is not reported")

	clips.failing["b"] = false

	assert.Equal(
		t,
		[]string{"b"},
		updatedIDs(t, feed.check(t.Context())),
		"the next check reports it",
	)
}

// asViewpoint returns the viewpoint of a caller with a scope and no Plex
// token.
//
// Parameters:
//   - scope: Whose clips the caller may change.
//
// Returns:
//   - viewpoint: The caller's viewpoint.
func asViewpoint(scope clipdom.Scope) middleware.Viewpoint {
	return middleware.Viewpoint{
		Viewer: identity.Viewer{Token: "", PlexID: scope.Owner, Owner: false},
		Scope:  scope,
	}
}

func TestLiveClipsFollowsNothingWithoutIDs(t *testing.T) {
	t.Parallel()

	handler, _ := clipPageHandler(t)

	assert.Nil(t, handler.LiveClips("", asViewpoint(clipdom.Scope{Owner: 42, Everyone: true})))
	assert.Nil(t, handler.LiveClips(" , ", asViewpoint(clipdom.Scope{Owner: 42, Everyone: true})))
}

// TestLiveClipsReadsFilesOnlyWhenTheStateChanges covers the storage lookups
// behind a region.
//
// On S3 each lookup is a request, so the feed makes one per status and
// stage, not one per check.
func TestLiveClipsReadsFilesOnlyWhenTheStateChanges(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/live.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := rendering("clip-1")

	job.OutputPath = "/clips/clip-1.mp4"

	jobQueue := queueForTest(t, job)

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Exists(mock.Anything, job.OutputPath).Return(true).Twice()

	handler := New(jobQueue, store, blob.Paths{}, db, &config.Config{MaxClipDur: time.Minute}, nil)
	check := handler.LiveClips(job.ID, asViewpoint(clipdom.Scope{Owner: 42, Everyone: true}))
	require.NotNil(t, check)

	first := check(t.Context())
	require.Len(t, first, 1)
	assert.Contains(t, first[0], `clip-clip-1-status`)

	for percent := range 3 {
		jobQueue.SetProgress(job.ID, percent+1)
		check(t.Context())
	}

	require.True(t, jobQueue.Cancel(job.ID), "a settled render reads its files again")

	check(t.Context())
}

// TestLiveClipsNeverReportsAnotherAccountsClip covers a member's page naming
// a clip the owner made.
//
// The feed reports it no more than an id nothing uses, and never asks the
// storage about it.
func TestLiveClipsNeverReportsAnotherAccountsClip(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/live.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := rendering("owners-clip")

	job.OwnerID = middlewaretest.OwnerID

	// The storage mock expects nothing, so a lookup of the clip's files fails
	// the test.
	handler := New(
		queueForTest(t, job),
		storagemocks.NewMockBlob(t),
		blob.Paths{},
		db,
		&config.Config{MaxClipDur: time.Minute},
		nil,
	)

	check := handler.LiveClips(
		job.ID,
		asViewpoint(clipdom.Scope{Owner: middlewaretest.MemberID, Everyone: false}),
	)
	require.NotNil(t, check)

	assert.Empty(t, check(t.Context()))
	assert.Empty(t, check(t.Context()))
}

// memberSources returns a media source whose bound server shares title 5
// with the member, whose token is member-token, and no other title.
//
// Parameters:
//   - t: The test the source belongs to.
//
// Returns:
//   - sources: The media source.
func memberSources(t *testing.T) *library.MediaSource {
	t.Helper()

	stub := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/library/metadata/5" {
				writer.WriteHeader(http.StatusNotFound)

				return
			}

			writer.Header().Set("Content-Type", "application/json")

			_, _ = writer.Write([]byte(
				`{"MediaContainer":{"Metadata":[{"ratingKey":"5","title":"Shared","type":"movie"}]}}`,
			))
		}),
	)
	t.Cleanup(stub.Close)

	server, ok := plex.ParseServerURL(stub.URL)
	require.True(t, ok)

	server.Token = "member-token"

	selected := librarymocks.NewMockServerSelection(t)
	selected.EXPECT().
		ClientWithToken("member-token").
		Return(plex.NewClient(plex.ClientConfig{
			Product: "outtake", ClientID: "test", Token: "member-token",
		}), server, true).
		Maybe()

	return library.NewMediaSource(&config.Config{}, selected, nil)
}

// TestLiveClipsReportsASharedClipOnlyWhenItsTitleIsSeen covers a member's
// page following two of the owner's shared clips.
//
// The clip whose title Plex shares with the member is reported, read only.
// The other is reported no more than an id nothing uses.
func TestLiveClipsReportsASharedClipOnlyWhenItsTitleIsSeen(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/live.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	seen := rendering("shared-seen")

	seen.OwnerID = middlewaretest.OwnerID
	seen.MediaID = "5"
	seen.Shared = true

	hidden := rendering("shared-hidden")

	hidden.OwnerID = middlewaretest.OwnerID
	hidden.MediaID = "6"
	hidden.Shared = true

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Exists(mock.Anything, mock.Anything).Return(false).Maybe()

	handler := New(
		queueForTest(t, seen, hidden),
		store,
		blob.Paths{},
		db,
		&config.Config{MaxClipDur: time.Minute},
		memberSources(t),
	)

	member := middleware.Viewpoint{
		Viewer: identity.Viewer{
			Token:  "member-token",
			PlexID: middlewaretest.MemberID,
			Owner:  false,
		},
		Scope: clipdom.Scope{Owner: middlewaretest.MemberID, Everyone: false},
	}

	check := handler.LiveClips("shared-seen,shared-hidden", member)
	require.NotNil(t, check)

	assert.Equal(t, []string{"shared-seen"}, updatedIDs(t, check(t.Context())))

	payloads := handler.LiveClips("shared-seen", member)(t.Context())
	require.Len(t, payloads, 1)
	assert.NotContains(t, payloads[0], "/cancel", "another account's render cannot be canceled")
}
