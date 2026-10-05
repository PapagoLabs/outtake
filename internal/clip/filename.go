// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"strings"
)

// DisplayName prefers a supplied clip name, then the media title.
//
// Parameters:
//   - name: Clip name, which may be empty.
//   - mediaTitle: Title of the source the clip was cut from.
//
// Returns:
//   - name: The clip name, or the media title when no name was set.
func DisplayName(name, mediaTitle string) string {
	if name != "" {
		return name
	}

	return mediaTitle
}

// Filename is the name a finished clip is offered under, which falls back to
// the clip id when neither the clip nor its source is named, and carries the
// extension its type is delivered in.
//
// Parameters:
//   - clip: The completed clip.
//
// Returns:
//   - filename: The filename the clip is delivered under.
func (clip *Clip) Filename() string {
	base := DisplayName(clip.Name, clip.MediaTitle)
	if base == "" {
		base = clip.ID
	}

	base = strings.NewReplacer("/", "-", `\`, "-").Replace(base)

	return base + fileExtension(clip.Type)
}

// fileExtension is the container a clip type is delivered in.
//
// Parameters:
//   - kind: Type of artifact the clip produces.
//
// Returns:
//   - extension: The file extension, including its leading dot.
func fileExtension(kind Type) string {
	switch kind {
	case TypeGIF:
		return ".gif"
	case TypeScreenshot:
		return ".jpg"
	default:
		return ".mp4"
	}
}
