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
}

const (
	// idFieldSeparator is the byte terminating each field hashed into a preview id.
	idFieldSeparator byte = 0x00

	// decimalBase is the base integers are rendered in before hashing.
	decimalBase = 10
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
// Parameters:
//   - req: Parsed request carrying the window and encoding options.
//   - source: Resolved source path.
//
// Returns:
//   - previewID: The preview id, within the allowlist ValidID checks.
//   - err: Non-nil when the source file cannot be read.
func RequestID(req clip.Request, source string) (string, error) {
	modTime, size, ok := sourceIdentity(source)
	if !ok {
		return "", ErrSourceUnreadable
	}

	return ContentID(KeyInput{
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
	}), nil
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
