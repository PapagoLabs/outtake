// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package respond

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/playback"
	"github.com/PapagoLabs/outtake/internal/clip/preview"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

var (
	// errDialRefused is a network failure reaching Plex.
	errDialRefused = errors.New("refused")

	// errUnnamed is a failure no rule names.
	errUnnamed = errors.New("disk full")
)

// TestMessageForNamesEachFailure covers the plain message for each failure a
// user can hit, as the wording review lists them, reached through the chain
// a handler wraps it in, and whether it refuses the user's input, which
// keeps details off it.
func TestMessageForNamesEachFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		err   error
		want  string
		input bool
	}{
		{
			name:  "a mark that is not a time",
			err:   &timecode.FieldError{Field: "start", Value: "soon"},
			want:  "Enter the start as a time, such as 00:01:23.456",
			input: true,
		},
		{
			name:  "a body that is not JSON",
			err:   api.ErrInvalidBody,
			want:  "The request body isn't valid JSON",
			input: true,
		},
		{
			name:  "an unknown type",
			err:   clip.ErrUnknownType,
			want:  "Choose Video Clip, GIF, or Screenshot",
			input: true,
		},
		{
			name:  "a profile that is gone",
			err:   profile.ErrUnknownProfile,
			want:  "That profile no longer exists. Choose another.",
			input: true,
		},
		{
			name:  "no server chosen",
			err:   library.ErrNoServer,
			want:  "Choose a Plex server under Servers first",
			input: true,
		},
		{
			name: "Plex refusing the token",
			err:  fmt.Errorf("get media path: %w", plex.ErrUnauthorized),
			want: "Plex rejected the saved login for this server. Logout and login again.",
		},
		{
			name: "a title Plex has no file for",
			err:  plex.ErrNoFilePathFound,
			want: "Couldn't find this title's file in Plex",
		},
		{
			name: "Plex unreachable",
			err:  &net.OpError{Op: "dial", Err: errDialRefused},
			want: "Couldn't reach the Plex server",
		},
		{
			name: "an unreadable source",
			err:  preview.ErrSourceUnreadable,
			want: "Outtake can't read this title's file. Check the media path setting.",
		},
		{
			name:  "an end not after the start",
			err:   clip.ErrEmptyRange,
			want:  "The end must be after the start",
			input: true,
		},
		{
			name:  "a negative start",
			err:   clip.ErrNegativeStart,
			want:  "The start can't be negative",
			input: true,
		},
		{
			name:  "a negative length",
			err:   clip.ErrNegativeLength,
			want:  "The length can't be negative",
			input: true,
		},
		{
			name:  "a selection over the limit",
			err:   &clip.SelectionTooLongError{Limit: 10 * time.Minute},
			want:  "A clip can be at most 10min",
			input: true,
		},
		{
			name: "a start past the end",
			err: &clip.PastSourceEndError{
				Edge:         clip.MarkStart,
				Mark:         time.Hour,
				SourceLength: 30 * time.Minute,
			},
			want:  "The start is past the end of the title (30min)",
			input: true,
		},
		{
			name: "an end past the end",
			err: &clip.PastSourceEndError{
				Edge:         clip.MarkEnd,
				Mark:         time.Hour,
				SourceLength: 30 * time.Minute,
			},
			want:  "The end is past the end of the title (30min)",
			input: true,
		},
		{
			name:  "a GIF too wide",
			err:   clip.ErrInvalidGIFWidth,
			want:  "GIF width must be 120 to 1920 px",
			input: true,
		},
		{
			name:  "a GIF frame rate out of range",
			err:   clip.ErrInvalidGIFFPS,
			want:  "GIF frame rate must be 5 to 30 fps",
			input: true,
		},
		{
			name:  "a missing audio track",
			err:   &clip.MissingAudioTrackError{Track: 3, Count: 2},
			want:  "This title has no audio track 3",
			input: true,
		},
		{
			name:  "a Dolby Vision source",
			err:   library.ErrDolbyVisionBaseLayer,
			want:  "This Dolby Vision file can't be exported with correct colors. Use a copy with an HDR10 base layer.",
			input: true,
		},
		{
			name:  "a clip that is rendering",
			err:   fmt.Errorf("edit c1: %w", queue.ErrJobActive),
			want:  "This clip is rendering. Wait for it or cancel it first.",
			input: true,
		},
		{
			name: "a server shutting down",
			err:  queue.ErrQueueStopped,
			want: "Outtake is shutting down. Try again once it's back.",
		},
		{
			name: "a clip that is gone",
			err:  catalog.ErrClipNotFound,
			want: "This clip no longer exists",
		},
		{
			name: "a stored clip that is gone",
			err:  database.ErrClipNotFound,
			want: "This clip no longer exists",
		},
		{
			name:  "too many previews",
			err:   preview.ErrBusy,
			want:  "Too many previews are rendering. Try again shortly.",
			input: true,
		},
		{
			name:  "a profile without a name",
			err:   profile.ErrProfileName,
			want:  "Enter a name",
			input: true,
		},
		{
			name:  "a profile name too long",
			err:   profile.ErrProfileNameLength,
			want:  "Names can be up to 64 characters",
			input: true,
		},
		{
			name:  "a CRF out of range",
			err:   profile.ErrProfileCRF,
			want:  "CRF must be 0 to 51",
			input: true,
		},
		{
			name:  "an unknown preset",
			err:   profile.ErrProfilePreset,
			want:  "Choose an encoder preset from the list",
			input: true,
		},
		{
			name:  "an audio bitrate out of range",
			err:   profile.ErrProfileAudio,
			want:  "Audio bitrate must be 64 to 640 kbps",
			input: true,
		},
		{
			name:  "an unknown resolution",
			err:   profile.ErrProfileWidth,
			want:  "Choose a maximum resolution from the list",
			input: true,
		},
		{
			name:  "a profile name in use",
			err:   fmt.Errorf("save clip profile: %w", database.ErrDuplicateClipProfileName),
			want:  "A profile with that name already exists",
			input: true,
		},
		{
			name:  "the last profile",
			err:   database.ErrLastClipProfile,
			want:  "The last profile can't be deleted",
			input: true,
		},
		{
			name: "a stored profile that is gone",
			err:  database.ErrClipProfileNotFound,
			want: "That profile no longer exists",
		},
		{
			name:  "an unknown preview maximum",
			err:   playback.ErrUnknownMaxPreviewWidth,
			want:  "Choose 720p, 1080p, or 4K",
			input: true,
		},
		{name: "anything else", err: errUnnamed, want: MessageUnexpected},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("handler: %w", test.err)

			message, input := classify(wrapped)
			assert.Equal(t, test.want, message)
			assert.Equal(t, test.input, input)
			assert.Equal(t, test.want, MessageFor(wrapped))
		})
	}
}
