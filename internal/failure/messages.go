// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package failure

import (
	"errors"
	"net"
	"os/exec"
	"strconv"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/clip/playback"
	"github.com/PapagoLabs/outtake/internal/clip/preview"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

// messageRule names the plain message for errors that match it.
type messageRule struct {
	// target is the sentinel the rule matches with errors.Is.
	target error
	// message is what the page says.
	message string
	// input marks a refusal of what the user entered, which the message
	// explains in full, so it carries no details and writes no log line.
	input bool
}

const (
	// MessageUnexpected is what the page says for a failure no rule names.
	MessageUnexpected = "Something went wrong. Check the Outtake log."
	// NotFoundMessage is what the page says for a clip nothing is registered
	// under.
	NotFoundMessage = "This clip no longer exists"
	// messageSourceUnreadable is what the page says when a title's file
	// cannot be read.
	messageSourceUnreadable = "Outtake can't read this title's file. Check the media path setting."
	// messageRenderFailed is what the page says when FFmpeg exits with an
	// error, whose output goes to the log.
	messageRenderFailed = "FFmpeg couldn't render this. Its output is in the Outtake log."
	// messageProfileGone is what the page says for a profile that was deleted.
	messageProfileGone = "That profile no longer exists"
	// messagePlexUnreachable is what the page says when Plex cannot be reached.
	messagePlexUnreachable = "Couldn't reach the Plex server"
)

// messageRules lists the plain message for each failure a user can hit,
// checked in order, so a narrower sentinel comes before the one it wraps.
var messageRules = []messageRule{
	{target: api.ErrInvalidBody, message: "The request body isn't valid JSON", input: true},
	{
		target:  clip.ErrHDRIsAProfileSetting,
		message: clip.ErrHDRIsAProfileSetting.Error(),
		input:   true,
	},
	{target: clip.ErrUnknownType, message: "Choose Video Clip, GIF, or Screenshot", input: true},
	{target: clip.ErrEmptyRange, message: "The end must be after the start", input: true},
	{target: clip.ErrNegativeStart, message: "The start can't be negative", input: true},
	{target: clip.ErrNegativeLength, message: "The length can't be negative", input: true},
	{target: clip.ErrInvalidDuration, message: "That length isn't allowed", input: true},
	{target: clip.ErrRangeOutsideMedia, message: "The selection is outside the title", input: true},
	{target: clip.ErrInvalidGIFWidth, message: "GIF width must be 120 to 1920 px", input: true},
	{target: clip.ErrInvalidGIFFPS, message: "GIF frame rate must be 5 to 30 fps", input: true},
	{
		target:  clip.ErrNegativeAudioTrack,
		message: "Choose an audio track from the list",
		input:   true,
	},
	{
		target:  clip.ErrNoSuchAudioTrack,
		message: "This title doesn't have that audio track",
		input:   true,
	},
	{
		target: library.ErrDolbyVisionBaseLayer,
		message: "This Dolby Vision file can't be exported with correct colors. " +
			"Use a copy with an HDR10 base layer.",

		input: true,
	},
	{target: library.ErrNoServer, message: "Choose a Plex server under Servers first", input: true},
	{
		target:  identity.ErrNoServer,
		message: "Choose a Plex server under Servers first",
		input:   true,
	},
	{
		target:  plex.ErrUnauthorized,
		message: "Plex rejected the saved login for this server. Logout and login again.",
	},
	{target: plex.ErrNoFilePathFound, message: "Couldn't find this title's file in Plex"},
	{target: plex.ErrInvalidMediaID, message: "Couldn't find this title's file in Plex"},
	{target: plex.ErrServerReturnedError, message: "The Plex server answered with an error"},
	{target: plex.ErrPlexError, message: "The Plex server answered with an error"},
	{target: preview.ErrSourceUnreadable, message: messageSourceUnreadable},
	{target: clip.ErrSourceUnreadable, message: messageSourceUnreadable},
	{
		target:  ffmpeg.ErrTimeout,
		message: "The render ran past its time limit. Raise OUTTAKE_FFMPEG_TIMEOUT_SEC to allow longer.",
	},
	{target: ffmpeg.ErrEmptyOutput, message: "The render produced an empty file"},
	{
		target:  ffmpeg.ErrNotStarted,
		message: "Outtake can't run FFmpeg. Check OUTTAKE_FFMPEG_PATH.",
	},
	{
		target:  preview.ErrBusy,
		message: "Too many previews are rendering. Try again shortly.",
		input:   true,
	},
	{target: queue.ErrQueueStopped, message: "Outtake is shutting down. Try again once it's back."},
	{
		target:  queue.ErrJobActive,
		message: "This clip is rendering. Wait for it or cancel it first.",
		input:   true,
	},
	{target: queue.ErrJobNotFound, message: NotFoundMessage},
	{target: catalog.ErrClipNotFound, message: NotFoundMessage},
	{target: database.ErrClipNotFound, message: NotFoundMessage},
	{
		target:  profile.ErrUnknownProfile,
		message: "That profile no longer exists. Choose another.",
		input:   true,
	},
	{target: profile.ErrProfileName, message: "Enter a name", input: true},
	{
		target:  profile.ErrProfileNameLength,
		message: "Names can be up to 64 characters",
		input:   true,
	},
	{target: profile.ErrProfileCRF, message: "CRF must be 0 to 51", input: true},
	{
		target:  profile.ErrProfilePreset,
		message: "Choose an encoder preset from the list",
		input:   true,
	},
	{target: profile.ErrProfileAudio, message: "Audio bitrate must be 64 to 640 kbps", input: true},
	{
		target:  profile.ErrProfileWidth,
		message: "Choose a maximum resolution from the list",
		input:   true,
	},
	{
		target:  database.ErrDuplicateClipProfileName,
		message: "A profile with that name already exists",
		input:   true,
	},
	{
		target:  database.ErrLastClipProfile,
		message: "The last profile can't be deleted",
		input:   true,
	},
	{target: database.ErrClipProfileNotFound, message: messageProfileGone},
	{target: playback.ErrUnknownMaxPreviewWidth, message: "Choose 720p, 1080p, or 4K", input: true},
}

// MessageFor is the plain message a page shows for err.
//
// Parameters:
//   - err: What went wrong.
//
// Returns:
//   - message: The plain message.
func MessageFor(err error) string {
	message, _ := Classify(err)

	return message
}

// Classify finds the plain message for err and whether it refuses what the
// user entered. A failure that carries a value the message names, such as the
// clip length limit, is matched by its type first, and every one of those is
// an input refusal. A network failure reaching Plex is matched by its type
// too. Anything else unnamed reads as MessageUnexpected.
//
// Parameters:
//   - err: What went wrong.
//
// Returns:
//   - message: The plain message.
//   - input: True when the message explains a refusal of the user's input.
//
//nolint:nonamedreturns // Two results read clearer named.
func Classify(err error) (message string, input bool) {
	if valued, ok := valueMessage(err); ok {
		return valued, true
	}

	for _, rule := range messageRules {
		if errors.Is(err, rule.target) {
			return rule.message, rule.input
		}
	}

	//nolint:errcheck // Only the match counts, since the matched value is err itself.
	if _, ok := errors.AsType[net.Error](err); ok {
		return messagePlexUnreachable, false
	}

	//nolint:errcheck // Only the match counts, since the matched value is err itself.
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return messageRenderFailed, false
	}

	return MessageUnexpected, false
}

// valueMessage names the failures whose message carries a value.
//
// Parameters:
//   - err: What went wrong.
//
// Returns:
//   - message: The plain message with its value.
//   - ok: True when err is one of these failures.
//
//nolint:nonamedreturns // Same-type returns need names.
func valueMessage(err error) (message string, ok bool) {
	if field, isField := errors.AsType[*timecode.FieldError](err); isField {
		return "Enter the " + field.Field + " as a time, such as 00:01:23.456", true
	}

	if long, isLong := errors.AsType[*clip.SelectionTooLongError](err); isLong {
		return "A clip can be at most " + timecode.FromDuration(long.Limit).Short(), true
	}

	if past, isPast := errors.AsType[*clip.PastSourceEndError](err); isPast {
		return "The " + past.Edge + " is past the end of the title (" +
			timecode.FromDuration(past.SourceLength).Short() + ")", true
	}

	if track, isTrack := errors.AsType[*clip.MissingAudioTrackError](err); isTrack {
		return "This title has no audio track " + strconv.Itoa(track.Track), true
	}

	return "", false
}
