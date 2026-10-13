// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"slices"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// Audience is who a page shows clip cards to.
type Audience struct {
	// Names are the installation's usernames by plex.tv id, nil when they
	// could not be read.
	Names map[int]string
	// Scope is whose clips the viewer may change.
	Scope clip.Scope
}

// FormerUser names the owner of a clip whose account was removed.
const FormerUser = "Former user"

// NewAudience builds the audience of a page from the usernames it read.
//
// Usernames that could not be read leave every card without an owner's name
// and offer no sharing, rather than failing the page.
//
// Parameters:
//   - scope: Whose clips the viewer may change.
//   - names: The installation's usernames by plex.tv id.
//   - err: Why the usernames could not be read, or nil.
//
// Returns:
//   - audience: The audience.
func NewAudience(scope clip.Scope, names map[int]string, err error) Audience {
	if err != nil {
		names = nil
	}

	return Audience{Names: names, Scope: scope}
}

// Apply marks what the viewer may do with a clip and whose it is.
//
// A clip the viewer may not change is read only. A clip they may change can
// be shared once the installation has another user to share it with. Another
// account's clip names its owner.
//
// Parameters:
//   - item: The card, changed in place.
//   - owner: The plex.tv id of the account that made the clip.
func (audience Audience) Apply(item *ClipItem, owner int) {
	item.ReadOnly = !audience.Scope.Includes(owner)
	item.CanShare = !item.ReadOnly && audience.Shares()
	item.OwnerName = ""

	if owner == audience.Scope.Owner || audience.Names == nil {
		return
	}

	name, ok := audience.Names[owner]
	if !ok {
		name = FormerUser
	}

	item.OwnerName = name
}

// ApplyEach marks every card in a list.
//
// Parameters:
//   - items: The cards, changed in place, in the order of jobs.
//   - jobs: The clips the cards were built from.
func (audience Audience) ApplyEach(items []ClipItem, jobs []*clip.Job) {
	for index := range items {
		audience.Apply(&items[index], jobs[index].OwnerID)
	}
}

// Shares reports whether the installation has another user to share clips
// with.
//
// Returns:
//   - shares: True once a second user is known.
func (audience Audience) Shares() bool {
	return len(audience.Names) > 1
}

// AnySelectable reports whether a list holds a card its viewer may change,
// which its bulk actions can select.
//
// Parameters:
//   - items: The cards.
//
// Returns:
//   - selectable: True when any card is not read only.
func AnySelectable(items []ClipItem) bool {
	return slices.ContainsFunc(items, func(item ClipItem) bool { return !item.ReadOnly })
}
