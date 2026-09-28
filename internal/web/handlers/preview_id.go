// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/media"
)

// previewKeyInput is everything that determines a preview's bytes.
//
// Every field here changes the ffmpeg command, so a change to any of them has
// to produce a different id, or a stale preview is served instead of rendering.
type previewKeyInput struct {
	// MediaID is the Plex rating key the preview was requested for.
	MediaID string
	// Path is the resolved source file.
	Path string
	// ModTime is the source modification time.
	ModTime time.Time
	// Size is the source size in bytes.
	Size int64
	// Start is the seek offset in seconds.
	Start float64
	// Duration is the encoded window in seconds, already clamped.
	Duration float64
	// AudioIndex is the selected audio stream.
	AudioIndex int
	// Crop is whether black bars are trimmed.
	Crop bool
	// WebSafe is whether HDR is tone mapped.
	WebSafe bool
}

// previewIDFieldSeparator terminates each field hashed into a preview id.
//
// Fixed-width lengths alone would not separate the fields: a path of "ab" and a
// rating key of "ab" would otherwise hash the same as one long field.
const previewIDFieldSeparator byte = 0x00

// decimalBase is the base integers are rendered in before hashing.
const decimalBase = 10

// previewIDPattern is the safe subset of characters a storage id may contain.
//
// It is deliberately narrower than the ids in use today. Preview ids are
// content hashes, and an allowlist of unreserved URL characters admits one
// without having to be revisited when the id scheme changes. Anything outside it
// is rejected before the value is joined into a filesystem path.
var previewIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// validPreviewID reports whether an id is safe to use as a storage filename.
//
// The route parameter reaches this unescaped, so a value like `../secret` never
// arrives intact today, and the `.mp4` suffix would turn a bare `..` into an
// ordinary filename. Both are accidents of the current wiring rather than
// guarantees, and the `.mp4` suffix disappears the moment previews are keyed
// on a bare content hash. Rejecting the unsafe characters explicitly keeps the
// path safe under any of those changes.
//
// Parameters:
//   - id: Route parameter naming a stored object.
//
// Returns:
//   - safe: True when the id contains only unreserved characters.
func validPreviewID(id string) bool {
	return previewIDPattern.MatchString(id)
}

// previewSourceIdentity reads the file fields of a preview key.
//
// A source that cannot be stat'd has no stable identity, so a preview id built
// without one would be wrong the moment the file is replaced. Callers skip the
// preview entirely in that case, since ffmpeg would fail on it too.
//
// Parameters:
//   - path: Resolved source path.
//
// Returns:
//   - modTime: Source modification time.
//   - size: Source size in bytes.
//   - ok: False when the file cannot be stat'd.
func previewSourceIdentity(path string) (time.Time, int64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, 0, false
	}

	return info.ModTime(), info.Size(), true
}

// previewRequestID derives the id of the preview a request describes.
//
// The duration is clamped through media.PreviewDuration, the same function
// ExtractPreview hands to ffmpeg, so a clip longer than the cap shares an entry
// with one that is not. A key built from the requested duration would miss on
// every clip worth previewing.
//
// Parameters:
//   - req: Parsed request carrying the window and encoding options.
//   - path: Resolved source path.
//
// Returns:
//   - id: The preview id, within the allowlist validPreviewID checks.
//   - err: Non-nil when the source file cannot be read.
func previewRequestID(req api.ClipRequest, path string) (string, error) {
	modTime, size, ok := previewSourceIdentity(path)
	if !ok {
		return "", errSourceUnreadable
	}

	return previewContentID(previewKeyInput{
		MediaID:    req.MediaID,
		Path:       path,
		ModTime:    modTime,
		Size:       size,
		Start:      req.StartTime,
		Duration:   media.PreviewDuration(req.Duration),
		AudioIndex: req.AudioIndex,
		Crop:       req.CropBlackBars,
		WebSafe:    derefBool(req.WebSafeColor),
	}), nil
}

// previewContentID derives the id of a preview from what determines its bytes.
//
// Two requests that would run an identical ffmpeg command produce the same id,
// so the second finds the first render's file instead of encoding again. Times
// are rendered at the same millisecond resolution the ffmpeg arguments use, so
// two values that produce an identical command cannot fall into different keys.
//
// The id must be rebuildable from a request alone: it is what lets the server
// confirm that a client-supplied end mark belongs to the preview it came from.
//
// Parameters:
//   - input: Everything determining the preview's bytes.
//
// Returns:
//   - id: A 64 character hex digest, within the allowlist validPreviewID checks.
func previewContentID(input previewKeyInput) string {
	fields := [...]string{
		input.MediaID,
		input.Path,
		media.FormatSeconds(input.Start),
		media.FormatSeconds(input.Duration),
		strconv.FormatInt(input.Size, decimalBase),
		strconv.FormatBool(input.Crop),
		strconv.FormatBool(input.WebSafe),
		strconv.Itoa(input.AudioIndex),
		input.ModTime.UTC().Format(time.RFC3339Nano),
	}

	// A byte slice rather than a strings.Builder, whose writes return an error
	// that can never happen and would have to be discarded. The input is a few
	// hundred bytes, so letting it grow is not worth pre-sizing.
	var buf []byte

	for _, field := range fields {
		buf = append(buf, field...)
		buf = append(buf, previewIDFieldSeparator)
	}

	sum := sha256.Sum256(buf)

	return hex.EncodeToString(sum[:])
}
