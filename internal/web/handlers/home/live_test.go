// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/handlers/home/mocks"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/middleware/middlewaretest"
)

// TestLivePlaybackFollowsOnlyAMediaItemID covers a value that is not a media
// item id.
//
// Anything but up to 12 digits starts no feed.
func TestLivePlaybackFollowsOnlyAMediaItemID(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, mocks.NewMockPlexAuth(t), silentSources(t))

	for _, value := range []string{"", "abc", "42/../1", strings.Repeat("9", 13)} {
		assert.Nil(t, handler.LivePlayback(value, middleware.Viewpoint{}), value)
	}
}

// TestLivePlaybackReportsEachChange covers the panel the feed reports.
//
// The first check reports the panel, an unchanged one reports nothing, and a
// new position from the session monitor reports the panel again.
func TestLivePlaybackReportsEachChange(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Sessions().Return(playingSessions(struct {
		mediaID string
		offset  float64
	}{"42", 900})).Twice()
	auth.EXPECT().Sessions().Return(playingSessions(struct {
		mediaID string
		offset  float64
	}{"42", 960})).Once()

	handler, _ := pageHandler(t, auth, silentSources(t))

	check := handler.LivePlayback("42", middleware.Viewpoint{
		Viewer: identity.OwnerViewer(middlewaretest.OwnerID),
		Scope:  clip.Scope{Owner: middlewaretest.OwnerID, Everyone: true},
	})
	require.NotNil(t, check)

	first := check(t.Context())
	require.Len(t, first, 1)

	var update playbackUpdate

	require.NoError(t, json.Unmarshal([]byte(first[0]), &update))
	assert.Equal(t, "42", update.ID)
	assert.Contains(t, update.HTML, "00:15:00.000", "the panel shows the monitor's position")

	assert.Empty(t, check(t.Context()), "an unchanged panel is not sent again")

	moved := check(t.Context())
	require.Len(t, moved, 1)
	assert.Contains(t, moved[0], "00:16:00.000")
}
