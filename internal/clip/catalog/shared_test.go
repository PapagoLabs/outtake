// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// titleAsker answers which titles a member sees and counts each question.
type titleAsker struct {
	// visible are the titles the member sees.
	visible map[string]bool
	// asked counts the questions for each title.
	asked map[string]int
	// mu guards asked.
	mu sync.Mutex
}

// member is a member's scope in the sharing tests.
var member = clip.Scope{Owner: 7, Everyone: false}

// newTitleAsker creates an asker that sees the given titles.
//
// Parameters:
//   - visible: The titles the member sees.
//
// Returns:
//   - asker: The asker.
func newTitleAsker(visible ...string) *titleAsker {
	asker := &titleAsker{visible: make(map[string]bool), asked: make(map[string]int)}

	for _, mediaID := range visible {
		asker.visible[mediaID] = true
	}

	return asker
}

// sees answers one question and counts it.
//
// Parameters:
//   - mediaID: The title.
//
// Returns:
//   - seen: True for a title the member sees.
func (asker *titleAsker) sees(_ context.Context, mediaID string) bool {
	asker.mu.Lock()
	defer asker.mu.Unlock()

	asker.asked[mediaID]++

	return asker.visible[mediaID]
}

// ownedClip is a finished clip of one source that an account owns.
//
// Parameters:
//   - id: Clip id.
//   - owner: The owning account.
//   - mediaID: The source title.
//   - shared: Whether the clip is shared.
//
// Returns:
//   - job: The clip.
func ownedClip(id string, owner int, mediaID string, shared bool) *clip.Job {
	job := stored(id)

	job.OwnerID = owner
	job.MediaID = mediaID
	job.Shared = shared

	return job
}

// TestViewableAddsSharedClipsOfVisibleTitles covers the read lookup.
//
// A member reads their own clips without a Plex question, another account's
// shared clip only when they can see its title, and never another account's
// private clip, which is not asked about either. The change lookup still
// refuses every clip of another account.
func TestViewableAddsSharedClipsOfVisibleTitles(t *testing.T) {
	t.Parallel()

	mine := ownedClip("mine", 7, "1", false)
	sharedSeen := ownedClip("shared-seen", 42, "5", true)
	sharedHidden := ownedClip("shared-hidden", 42, "6", true)
	private := ownedClip("private", 42, "5", false)
	store := sourceTestDB(t, mine, sharedSeen, sharedHidden, private)
	work := sourceTestQueue(t)
	asker := newTitleAsker("5")

	assert.NotNil(t, Viewable(t.Context(), work, store, member, asker.sees, "mine"))
	assert.NotNil(t, Viewable(t.Context(), work, store, member, asker.sees, "shared-seen"))
	assert.Nil(t, Viewable(t.Context(), work, store, member, asker.sees, "shared-hidden"))
	assert.Nil(t, Viewable(t.Context(), work, store, member, asker.sees, "private"))
	assert.Nil(t, Viewable(t.Context(), work, store, member, nil, "shared-seen"),
		"without a way to ask Plex, another account's clip stays hidden")
	assert.Nil(t, Viewable(t.Context(), work, store, member, asker.sees, "absent"))

	assert.Equal(t, map[string]int{"5": 1, "6": 1}, asker.asked,
		"only another account's shared clips are asked about")

	assert.Nil(t, Job(t.Context(), work, store, member, "shared-seen"),
		"a shared clip is still not the member's to change")
}

// TestSharedListsEveryVisibleSharedClip covers the Shared filter.
//
// A member gets their own shared clips and another account's shared clips of
// titles they can see, asking about each title once. The owner and admins get
// every shared clip without asking.
func TestSharedListsEveryVisibleSharedClip(t *testing.T) {
	t.Parallel()

	clips := []*clip.Job{
		ownedClip("mine-shared", 7, "1", true),
		ownedClip("mine-private", 7, "1", false),
		ownedClip("seen-a", 42, "5", true),
		ownedClip("seen-b", 42, "5", true),
		ownedClip("hidden", 42, "6", true),
		ownedClip("private", 42, "5", false),
	}
	store := sourceTestDB(t, clips...)
	asker := newTitleAsker("5")

	assert.ElementsMatch(
		t,
		[]string{"mine-shared", "seen-a", "seen-b"},
		clipJobIDs(Shared(t.Context(), sourceTestQueue(t), store, member, asker.sees)),
	)
	assert.Equal(t, map[string]int{"5": 1, "6": 1}, asker.asked)

	assert.ElementsMatch(
		t,
		[]string{"mine-shared", "seen-a", "seen-b", "hidden"},
		clipJobIDs(Shared(t.Context(), sourceTestQueue(t, clips...), store, everyone, nil)),
		"the owner sees every shared clip, read from the queue",
	)
}

// TestSharedForMediaListsOnlyOtherAccountsClips covers a title's Shared Clips
// section.
//
// It holds other accounts' shared clips of the title when the member can see
// it, and nothing for the owner, whose own list already holds every clip.
func TestSharedForMediaListsOnlyOtherAccountsClips(t *testing.T) {
	t.Parallel()

	store := sourceTestDB(t,
		ownedClip("mine-shared", 7, "5", true),
		ownedClip("theirs", 42, "5", true),
		ownedClip("theirs-private", 42, "5", false),
		ownedClip("elsewhere", 42, "6", true),
	)

	assert.Equal(
		t,
		[]string{"theirs"},
		clipJobIDs(SharedForMedia(t.Context(), store, member, newTitleAsker("5").sees, "5")),
	)
	assert.Empty(t, SharedForMedia(t.Context(), store, member, newTitleAsker().sees, "5"),
		"a title the member cannot see shows none")
	assert.Empty(t, SharedForMedia(t.Context(), store, everyone, nil, "5"))

	require.NoError(t, store.Close())

	assert.Nil(t, SharedForMedia(t.Context(), store, member, newTitleAsker("5").sees, "5"),
		"an unreachable database yields nothing")
}

// TestSeenTitlesAsksAFewAtATime covers the bound on concurrent Plex
// questions.
//
// With every question held, exactly titleCheckLimit are under way, and every
// title is answered once they are let go.
func TestSeenTitlesAsksAFewAtATime(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var running atomic.Int32

		release := make(chan struct{})
		titles := make([]string, 0, titleCheckLimit*3)

		for index := range titleCheckLimit * 3 {
			titles = append(titles, strconv.Itoa(index))
		}

		done := make(chan map[string]bool)

		go func() {
			done <- seenTitles(t.Context(), titles, func(context.Context, string) bool {
				running.Add(1)
				defer running.Add(-1)

				<-release

				return true
			})
		}()

		synctest.Wait()
		assert.Equal(t, int32(titleCheckLimit), running.Load())

		close(release)

		assert.Len(t, <-done, len(titles))
	})
}

// TestSeenTitlesStopsOnceTheRequestEnds covers a request that ends while
// titles wait for a slot.
//
// The checks under way finish, no other check starts, and every title left
// unasked reads as not seen.
func TestSeenTitlesStopsOnceTheRequestEnds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var asked atomic.Int32

		ctx, cancel := context.WithCancel(t.Context())
		release := make(chan struct{})
		titles := make([]string, 0, titleCheckLimit*3)

		for index := range titleCheckLimit * 3 {
			titles = append(titles, strconv.Itoa(index))
		}

		done := make(chan map[string]bool)

		go func() {
			done <- seenTitles(ctx, titles, func(context.Context, string) bool {
				asked.Add(1)
				<-release

				return true
			})
		}()

		synctest.Wait()
		cancel()

		// The loop leaves before any slot is freed, so the end decides.
		synctest.Wait()
		close(release)

		seen := <-done

		assert.Equal(t, int32(titleCheckLimit), asked.Load(), "no check starts after the end")
		assert.Len(t, seen, titleCheckLimit)
	})

	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	assert.Empty(t, seenTitles(canceled, []string{"5"}, func(context.Context, string) bool {
		t.Error("a check started after the request ended")

		return true
	}))
}

// TestShareChangesWhetherAClipIsShared covers sharing a clip and making it
// private again, whether the queue or only storage holds it.
//
// The queue's report saves the change, and a clip neither holds is not found.
func TestShareChangesWhetherAClipIsShared(t *testing.T) {
	t.Parallel()

	store := sourceTestDB(t, ownedClip("stored", 7, "5", false))
	work := sourceTestQueue(t)
	work.SetStatusFunc(func(job *clip.Job) {
		_ = store.SaveClip(t.Context(), job)
	})

	job, err := Share(t.Context(), work, store, "stored", true)
	require.NoError(t, err)
	assert.True(t, job.Shared)
	assert.True(t, work.GetJob("stored").Shared, "the queue holds the change")

	saved, err := store.GetClip(t.Context(), "stored")
	require.NoError(t, err)
	assert.True(t, saved.Shared, "the queue's report saved it")

	job, err = Share(t.Context(), work, store, "stored", false)
	require.NoError(t, err)
	assert.False(t, job.Shared)

	_, err = Share(t.Context(), work, store, "absent", true)
	require.ErrorIs(t, err, ErrClipNotFound)
}
