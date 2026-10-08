// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"strconv"
	"strings"

	"github.com/PapagoLabs/outtake/internal/plex/library"
)

const (
	// undetermined is how ffprobe reports a field it has nothing to say about.
	undetermined = "und"

	// labelSeparator joins the parts of an audio track label.
	labelSeparator = " · "

	// trackFallback is the label prefix for a track that describes itself in no
	// way at all.
	trackFallback = "Track "
)

// ApplySource fills in what a clip card knows from probing its source: the
// audio tracks it offers, whether it shows the HDR choice, and the source
// length the window is checked against. A source that could not be probed
// leaves them empty.
//
// Parameters:
//   - item: The card to fill in.
//   - source: What probing the clip's source turned up.
func ApplySource(item *ClipItem, source library.SourceInfo) {
	item.AudioTracks = AudioTrackOptions(source.AudioStreams)
	item.SourceHDR = source.HDR
	item.MediaDuration = source.Duration
}

// AudioTrackOptions maps probed audio streams onto select options.
//
// Parameters:
//   - streams: Probed audio streams.
//
// Returns:
//   - options: One form option per stream, in probe order.
func AudioTrackOptions(streams []library.AudioStream) []AudioTrackOption {
	options := make([]AudioTrackOption, 0, len(streams))

	for _, stream := range streams {
		options = append(options, AudioTrackOption{
			Index: stream.Index,
			Label: AudioTrackLabel(stream),
		})
	}

	return options
}

// AudioTrackLabel builds a short description of an audio stream.
//
// Parameters:
//   - stream: Probed audio stream.
//
// Returns:
//   - label: Language, codec, layout, and title joined for display.
func AudioTrackLabel(stream library.AudioStream) string {
	var parts []string

	if stream.Language != "" && stream.Language != undetermined {
		parts = append(parts, stream.Language)
	}

	if stream.Codec != "" {
		parts = append(parts, stream.Codec)
	}

	if stream.Layout != "" {
		parts = append(parts, stream.Layout)
	}

	if stream.Title != "" {
		parts = append(parts, stream.Title)
	}

	if len(parts) == 0 {
		return trackFallback + strconv.Itoa(stream.Index+1)
	}

	return strings.Join(parts, labelSeparator)
}
