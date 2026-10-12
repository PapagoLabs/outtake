// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// errNamesUnread stands in for usernames that could not be read.
var errNamesUnread = errors.New("database closed")

// TestAudienceApplyMarksWhatTheViewerMayDo covers each viewer and clip a
// card can pair.
//
// A member's own clip can be shared and names no owner. Another account's
// clip is read only and names its owner, or a former user once the account
// is gone. The owner may change a member's clip, which still names the
// member. Nothing can be shared while the installation has one user, and no
// owner is named when the names could not be read.
func TestAudienceApplyMarksWhatTheViewerMayDo(t *testing.T) {
	t.Parallel()

	names := map[int]string{42: "owner", 7: "member"}
	member := clip.Scope{Owner: 7, Everyone: false}
	owner := clip.Scope{Owner: 42, Everyone: true}

	for name, test := range map[string]struct {
		audience Audience
		want     ClipItem
		owner    int
	}{
		"own clip": {
			audience: Audience{Names: names, Scope: member},
			owner:    7,
			want:     ClipItem{CanShare: true},
		},
		"another account's clip": {
			audience: Audience{Names: names, Scope: member},
			owner:    42,
			want:     ClipItem{ReadOnly: true, OwnerName: "owner"},
		},
		"a removed account's clip": {
			audience: Audience{Names: names, Scope: member},
			owner:    9,
			want:     ClipItem{ReadOnly: true, OwnerName: FormerUser},
		},
		"the owner on a member's clip": {
			audience: Audience{Names: names, Scope: owner},
			owner:    7,
			want:     ClipItem{CanShare: true, OwnerName: "member"},
		},
		"one user": {
			audience: Audience{Names: map[int]string{42: "owner"}, Scope: owner},
			owner:    42,
			want:     ClipItem{},
		},
		"names unread": {
			audience: Audience{Names: nil, Scope: member},
			owner:    42,
			want:     ClipItem{ReadOnly: true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			item := ClipItem{OwnerName: "stale"}

			test.audience.Apply(&item, test.owner)

			assert.Equal(t, test.want, item)
		})
	}
}

// TestAudienceSharesOnceThereIsAnotherUser covers when sharing is offered.
func TestAudienceSharesOnceThereIsAnotherUser(t *testing.T) {
	t.Parallel()

	assert.False(t, Audience{}.Shares(), "names that could not be read offer nothing")
	assert.False(t, Audience{Names: map[int]string{42: "owner"}}.Shares())
	assert.True(t, Audience{Names: map[int]string{42: "owner", 7: "member"}}.Shares())
}

// TestNewAudienceDropsNamesItCouldNotRead covers a failed username read,
// which leaves the audience without names rather than failing the page.
func TestNewAudienceDropsNamesItCouldNotRead(t *testing.T) {
	t.Parallel()

	scope := clip.Scope{Owner: 7, Everyone: false}
	names := map[int]string{42: "owner", 7: "member"}

	assert.Equal(t, Audience{Names: names, Scope: scope}, NewAudience(scope, names, nil))
	assert.Equal(
		t,
		Audience{Names: nil, Scope: scope},
		NewAudience(scope, names, errNamesUnread),
	)
}

// TestAudienceApplyEachMarksEveryCard covers a list, whose cards follow the
// order of their clips.
func TestAudienceApplyEachMarksEveryCard(t *testing.T) {
	t.Parallel()

	audience := Audience{
		Names: map[int]string{42: "owner", 7: "member"},
		Scope: clip.Scope{Owner: 7, Everyone: false},
	}
	items := []ClipItem{{}, {}}

	audience.ApplyEach(items, []*clip.Job{{OwnerID: 7}, {OwnerID: 42}})

	assert.False(t, items[0].ReadOnly)
	assert.True(t, items[1].ReadOnly)
	assert.Equal(t, "owner", items[1].OwnerName)
}
