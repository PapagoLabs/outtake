// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

// KeyInput is everything that determines a preview's bytes.
type KeyInput struct {
	// MediaID is the Plex rating key the preview was requested for.
	MediaID string
	// Path is the resolved source file.
	Path string
	// ModTime is the source modification time.
	ModTime time.Time
	// Size is the source size in bytes.
	Size int64
	// Start is the seek offset.
	Start time.Duration
	// Duration is the encoded window, already clamped.
	Duration time.Duration
	// AudioIndex is the selected audio stream.
	AudioIndex int
	// Crop is whether black bars are trimmed.
	Crop bool
	// PreserveHDR is whether the source's HDR transfer is kept.
	PreserveHDR bool
	// MaxWidth is the widest the preview is scaled.
	MaxWidth int
}

const (
	// idFieldSeparator is the byte terminating each field hashed into a preview id.
	idFieldSeparator byte = 0x00

	// decimalBase is the base integers are rendered in before hashing.
	decimalBase = 10

	// ownerPrefix opens an owned preview id, before the owner's account id.
	ownerPrefix = "u"

	// ownerSeparator ends the owner's account id in an owned preview id.
	ownerSeparator = "-"

	// ownedDigestLength is how much of the content digest an owned preview id
	// keeps, so the owner's account id fits within ValidID's 64 characters.
	ownedDigestLength = 40
)

// idPattern is the safe subset of characters a storage id may contain.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ErrSourceUnreadable is reported when a preview's source cannot be read.
var ErrSourceUnreadable = errors.New("source file is not readable")

// ValidID reports whether an id is safe to use as a storage filename.
//
// Parameters:
//   - previewID: Route parameter naming a stored object.
//
// Returns:
//   - safe: True when the id contains only unreserved characters.
func ValidID(previewID string) bool {
	return idPattern.MatchString(previewID)
}

// sourceIdentity reads the file fields of a preview key.
//
// Parameters:
//   - source: Resolved source path.
//
// Returns:
//   - modTime: Source modification time.
//   - size: Source size in bytes.
//   - ok: False when the file cannot be stat'd.
func sourceIdentity(source string) (time.Time, int64, bool) {
	info, err := os.Stat(source)
	if err != nil {
		return time.Time{}, 0, false
	}

	return info.ModTime(), info.Size(), true
}

// RequestID derives the id of the preview a request describes.
//
// The id names the account that asked for it, so two accounts never share a
// preview and Owner can tell whose it is.
//
// Parameters:
//   - req: Parsed request carrying the window and encoding options.
//   - source: Resolved source path.
//   - maxWidth: The widest the preview is scaled, so a changed maximum
//     preview resolution renders a new preview.
//   - owner: The plex.tv account id of the caller, zero for none.
//
// Returns:
//   - previewID: The preview id, within the allowlist ValidID checks.
//   - err: Non-nil when the source file cannot be read.
func RequestID(req clip.Request, source string, maxWidth, owner int) (string, error) {
	modTime, size, ok := sourceIdentity(source)
	if !ok {
		return "", ErrSourceUnreadable
	}

	return OwnedID(owner, ContentID(KeyInput{
		MediaID: req.MediaID,
		Path:    source,
		ModTime: modTime,
		Size:    size,
		Start:   timecode.FromSeconds(req.StartTime).Duration(),
		Duration: ffmpeg.PreviewDuration(
			timecode.FromSeconds(req.Duration).Duration(),
		),
		AudioIndex:  req.AudioIndex,
		Crop:        req.CropBlackBars,
		PreserveHDR: clip.Flag(req.PreserveHDR),
		MaxWidth:    maxWidth,
	})), nil
}

// OwnedID names a preview after the account it belongs to and its content.
//
// Parameters:
//   - owner: The plex.tv account id of the preview's owner, zero for none.
//   - contentID: The preview's ContentID.
//
// Returns:
//   - previewID: The owner's id, then the start of the content digest, within
//     the allowlist ValidID checks.
func OwnedID(owner int, contentID string) string {
	return ownerPrefix + strconv.Itoa(
		max(owner, 0),
	) + ownerSeparator + contentID[:ownedDigestLength]
}

// Owner reads the account a preview id belongs to.
//
// Parameters:
//   - previewID: A preview id.
//
// Returns:
//   - owner: The owner's plex.tv account id, zero for none.
//   - ok: False for an id OwnedID did not make, whose owner is unknown.
func Owner(previewID string) (int, bool) {
	rest, found := strings.CutPrefix(previewID, ownerPrefix)
	if !found {
		return 0, false
	}

	digits, digest, found := strings.Cut(rest, ownerSeparator)
	if !found || len(digest) != ownedDigestLength {
		return 0, false
	}

	owner, err := strconv.Atoi(digits)
	if err != nil || owner < 0 {
		return 0, false
	}

	return owner, true
}

// ContentID derives the id of a preview from what determines its bytes.
//
// Parameters:
//   - input: Everything determining the preview's bytes.
//
// Returns:
//   - previewID: A 64 character hex digest, within the allowlist ValidID checks.
func ContentID(input KeyInput) string {
	fields := [...]string{
		input.MediaID,
		input.Path,
		timecode.FromDuration(input.Start).FormatSeconds(),
		timecode.FromDuration(input.Duration).FormatSeconds(),
		strconv.FormatInt(input.Size, decimalBase),
		strconv.FormatBool(input.Crop),
		strconv.FormatBool(input.PreserveHDR),
		strconv.Itoa(input.AudioIndex),
		strconv.Itoa(input.MaxWidth),
		input.ModTime.UTC().Format(time.RFC3339Nano),
	}

	// A byte slice rather than a strings.Builder, whose writes return an error
	// that can never happen and would have to be discarded.
	var buf []byte

	for _, field := range fields {
		buf = append(buf, field...)
		buf = append(buf, idFieldSeparator)
	}

	sum := sha256.Sum256(buf)

	return hex.EncodeToString(sum[:])
}
