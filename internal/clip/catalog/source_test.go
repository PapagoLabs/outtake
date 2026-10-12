// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// everyone is the scope of the owner or an admin, who sees every clip.
var everyone = clip.Scope{Owner: 42, Everyone: true}

// sourceTestDB returns an empty clip database with the given clips stored.
//
// Parameters:
//   - t: The test the database belongs to.
//   - clips: Clips to store.
//
// Returns:
//   - store: The database handle.
func sourceTestDB(t *testing.T, clips ...*clip.Job) *database.DB {
	t.Helper()

	store, err := database.New(filepath.Join(t.TempDir(), "source.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = store.Close() })

	for _, item := range clips {
		require.NoError(t, store.SaveClip(t.Context(), item))
	}

	return store
}

// sourceTestQueue returns a queue holding the given clips.
//
// Parameters:
//   - t: The test the queue belongs to.
//   - clips: Clips the queue should hold.
//
// Returns:
//   - work: The queue under test.
func sourceTestQueue(t *testing.T, clips ...*clip.Job) *queue.Queue {
	t.Helper()

	work := queue.NewQueue(1, nil)
	t.Cleanup(work.Stop)

	for _, item := range clips {
		work.Restore(item)
	}

	return work
}

// stored is a finished clip for the source-lookup tests.
//
// Parameters:
//   - id: Clip id.
//
// Returns:
//   - clip: The clip under test.
func stored(id string) *clip.Job {
	return &clip.Job{
		ID:         id,
		Type:       clip.TypeClip,
		MediaTitle: "Movie", Status: clip.StatusCompleted,
	}
}

func TestJobsPrefersTheQueue(t *testing.T) {
	t.Parallel()

	store := sourceTestDB(t, stored("in-storage"))
	work := sourceTestQueue(t, stored("in-flight"))

	got := Jobs(t.Context(), work, store, everyone)

	assert.Equal(t, []string{"in-flight"}, clipJobIDs(got),
		"a queued render has not been written yet, so the queue answers for it")
}

func TestJobsFallsBackToStorage(t *testing.T) {
	t.Parallel()

	store := sourceTestDB(t, stored("finished-a"), stored("finished-b"))

	got := Jobs(t.Context(), sourceTestQueue(t), store, everyone)

	assert.ElementsMatch(t, []string{"finished-a", "finished-b"}, clipJobIDs(got))
}

func TestJobsWithNothingToShow(t *testing.T) {
	t.Parallel()

	store := sourceTestDB(t)
	work := sourceTestQueue(t)

	assert.Empty(t, Jobs(t.Context(), work, store, everyone))

	require.NoError(t, store.Close())

	assert.Empty(t, Jobs(t.Context(), work, store, everyone),
		"an unreachable database yields no clips rather than an error")
}

func TestJobPrefersTheQueue(t *testing.T) {
	t.Parallel()

	store := sourceTestDB(t, stored("shared"))
	work := sourceTestQueue(t, stored("in-flight"))

	assert.Equal(
		t,
		clip.StatusCompleted,
		Job(t.Context(), work, store, everyone, "in-flight").Status,
		"the queue answers for a clip it holds",
	)
	assert.Nil(t, Job(t.Context(), work, store, everyone, "absent"))

	require.NoError(t, store.Close())

	assert.Nil(t, Job(t.Context(), work, store, everyone, "shared"),
		"an unreachable database yields nothing rather than an error")
}

func TestJobReadsStorageForAClipTheQueueDoesNotHold(t *testing.T) {
	t.Parallel()

	store := sourceTestDB(t, stored("finished"))

	got := Job(t.Context(), sourceTestQueue(t), store, everyone, "finished")

	require.NotNil(t, got)
	assert.Equal(t, "finished", got.ID)
}

func TestForMedia(t *testing.T) {
	t.Parallel()

	store := sourceTestDB(t)

	for _, id := range []string{"a", "b"} {
		item := stored(id)

		item.MediaID = "42"

		require.NoError(t, store.SaveClip(t.Context(), item))
	}

	other := stored("c")

	other.MediaID = "99"

	require.NoError(t, store.SaveClip(t.Context(), other))

	assert.ElementsMatch(
		t,
		[]string{"a", "b"},
		clipJobIDs(ForMedia(t.Context(), store, everyone, "42")),
	)
	assert.Empty(t, ForMedia(t.Context(), store, everyone, "absent"),
		"a source with no clips yields an empty list rather than an error")
}

// TestLookupsKeepAMemberToTheirOwnClips covers a member, who sees only the
// clips they made.
//
// Another account's clip is left out of lists and reads as missing by id,
// whether the queue or storage holds it.
func TestLookupsKeepAMemberToTheirOwnClips(t *testing.T) {
	t.Parallel()

	mine := stored("mine")

	mine.OwnerID = 7
	mine.MediaID = "42"

	theirs := stored("theirs")

	theirs.OwnerID = 42
	theirs.MediaID = "42"

	member := clip.Scope{Owner: 7, Everyone: false}
	store := sourceTestDB(t, mine, theirs)

	assert.Equal(
		t,
		[]string{"mine"},
		clipJobIDs(Jobs(t.Context(), sourceTestQueue(t), store, member)),
	)
	assert.Equal(
		t,
		[]string{"mine"},
		clipJobIDs(Jobs(t.Context(), sourceTestQueue(t, mine, theirs), store, member)),
	)
	assert.Equal(t, []string{"mine"}, clipJobIDs(ForMedia(t.Context(), store, member, "42")))

	assert.NotNil(t, Job(t.Context(), sourceTestQueue(t), store, member, "mine"))
	assert.Nil(t, Job(t.Context(), sourceTestQueue(t), store, member, "theirs"),
		"another account's stored clip reads as missing")
	assert.Nil(t, Job(t.Context(), sourceTestQueue(t, theirs), store, member, "theirs"),
		"and so does one the queue holds")
	assert.NotNil(t, Job(t.Context(), sourceTestQueue(t), store, everyone, "theirs"),
		"the owner and admins see every clip")
}
