// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// MediaInfo represents media file information.
type MediaInfo struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	VideoCodec string  `json:"video_codec"`
	AudioCodec string  `json:"audio_codec"`
	Format     string  `json:"format"`
	BitRate    int64   `json:"bit_rate"`
	// ColorTransfer is ffprobe color_transfer of the first video stream.
	ColorTransfer string       `json:"color_transfer"`
	AudioTracks   []AudioTrack `json:"audio_tracks"`
}

// AudioTrack is one audio stream on a source file.
type AudioTrack struct {
	Index    int
	Codec    string
	Language string
	Title    string
	Channels int
}

// probeFormat represents the format information from ffprobe.
type probeFormat struct {
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
	FormatName string `json:"format_name"`
}

// probeStream represents a stream from ffprobe.
type probeStream struct {
	Index         int       `json:"index"`
	CodecType     string    `json:"codec_type"`
	CodecName     string    `json:"codec_name"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	Channels      int       `json:"channels"`
	ColorTransfer string    `json:"color_transfer"`
	Tags          probeTags `json:"tags"`
}

// probeTags holds optional ffprobe stream tags.
type probeTags struct {
	Language string `json:"language"`
	Title    string `json:"title"`
	Name     string `json:"name"`
}

// probeOutput represents the output from ffprobe.
type probeOutput struct {
	Format  probeFormat   `json:"format"`
	Streams []probeStream `json:"streams"`
}

const (
	// bitRateBits is the bit size for integer parsing.
	bitRateBits = 64
	// decimalBase is the base 10 for integer parsing.
	decimalBase = 10
	// emptyVideoCodec is an empty video codec placeholder.
	emptyVideoCodec = ""
	// emptyAudioCodec is an empty audio codec placeholder.
	emptyAudioCodec = ""
)

// ParseOutput parses ffprobe JSON into a [MediaInfo] summary.
//
// Parameters:
//   - data: Raw stdout from ffprobe -print_format json.
//
// Returns:
//   - mediaInfo: Duration, codecs, dimensions, and audio tracks when present.
//   - err: Non-nil when data is not valid ffprobe JSON.
func ParseOutput(data []byte) (MediaInfo, error) {
	var output probeOutput

	err := json.Unmarshal(data, &output)
	if err != nil {
		return MediaInfo{}, fmt.Errorf("parse probe output: %w", err)
	}

	info := MediaInfo{
		Duration:    0,
		Width:       0,
		Height:      0,
		VideoCodec:  "",
		AudioCodec:  "",
		BitRate:     0,
		Format:      output.Format.FormatName,
		AudioTracks: nil,
	}

	parseDuration(output, &info)
	parseBitRate(output, &info)
	parseStreams(output, &info)

	return info, nil
}

// parseDuration copies format.duration onto info when it parses as seconds.
//
// Parameters:
//   - output: Decoded ffprobe JSON container.
//   - info: Destination for the parsed duration. Duration stays 0 when the
//     field is missing or invalid.
func parseDuration(output probeOutput, info *MediaInfo) {
	if output.Format.Duration == "" {
		return
	}

	dur, err := strconv.ParseFloat(output.Format.Duration, bitRateBits)
	if err == nil {
		info.Duration = dur
	}
}

// parseBitRate copies format.bit_rate onto info when it parses as an int.
//
// Parameters:
//   - output: Decoded ffprobe JSON container.
//   - info: Destination for the parsed bit rate. BitRate stays 0 when the field
//     is missing or invalid.
func parseBitRate(output probeOutput, info *MediaInfo) {
	if output.Format.BitRate == "" {
		return
	}

	br, err := strconv.ParseInt(output.Format.BitRate, decimalBase, bitRateBits)
	if err == nil {
		info.BitRate = br
	}
}

// parseStreams walks every stream and merges video/audio fields into info.
//
// Parameters:
//   - output: Decoded ffprobe JSON container.
//   - info: Destination updated in place for the first video and all audio.
func parseStreams(output probeOutput, info *MediaInfo) {
	for index := range output.Streams {
		parseStream(output.Streams[index], info)
	}
}

// parseStream applies one ffprobe stream to info (first video wins; audio appends).
//
// Parameters:
//   - stream: One entry from ffprobe streams[].
//   - info: Destination updated in place. Video fields fill once. Audio tracks
//     accumulate.
func parseStream(stream probeStream, info *MediaInfo) {
	switch stream.CodecType {
	case "video":
		if info.VideoCodec == emptyVideoCodec {
			info.VideoCodec = stream.CodecName
			info.Width = stream.Width
			info.Height = stream.Height
			info.ColorTransfer = stream.ColorTransfer
		}
	case "audio":
		if info.AudioCodec == emptyAudioCodec {
			info.AudioCodec = stream.CodecName
		}

		info.AudioTracks = append(info.AudioTracks, AudioTrack{
			Index:    len(info.AudioTracks),
			Codec:    stream.CodecName,
			Language: stream.Tags.Language,
			Title:    audioTitle(stream.Tags),
			Channels: stream.Channels,
		})
	default:
	}
}

// audioTitle prefers a stream title, then the handler name tag.
//
// Parameters:
//   - tags: ffprobe stream tags (title and name/handler_name).
//
// Returns:
//   - title: Non-empty stream title when set, otherwise the name/handler tag.
//     Empty when both title and name are unset.
func audioTitle(tags probeTags) string {
	if tags.Title != "" {
		return tags.Title
	}

	return tags.Name
}
