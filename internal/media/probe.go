// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// probeFormat represents the format information from ffprobe.
type probeFormat struct {
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
	FormatName string `json:"format_name"`
}

// probeStream represents a stream from ffprobe.
type probeStream struct {
	Index     int       `json:"index"`
	CodecType string    `json:"codec_type"`
	CodecName string    `json:"codec_name"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Channels  int       `json:"channels"`
	Tags      probeTags `json:"tags"`
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
	// BitRateBits is the bit size for integer parsing.
	bitRateBits = 64
	// DecimalBase is the base 10 for integer parsing.
	decimalBase = 10
	// EmptyVideoCodec is an empty video codec placeholder.
	emptyVideoCodec = ""
	// EmptyAudioCodec is an empty audio codec placeholder.
	emptyAudioCodec = ""
)

// parseProbeOutput parses the ffprobe output.
func parseProbeOutput(data []byte) (MediaInfo, error) {
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

// parseDuration parses the duration from the probe output.
func parseDuration(output probeOutput, info *MediaInfo) {
	if output.Format.Duration == "" {
		return
	}

	dur, err := strconv.ParseFloat(output.Format.Duration, bitRateBits)
	if err == nil {
		info.Duration = dur
	}
}

// parseBitRate parses the bit rate from the probe output.
func parseBitRate(output probeOutput, info *MediaInfo) {
	if output.Format.BitRate == "" {
		return
	}

	br, err := strconv.ParseInt(output.Format.BitRate, decimalBase, bitRateBits)
	if err == nil {
		info.BitRate = br
	}
}

// parseStreams parses the streams from the probe output.
func parseStreams(output probeOutput, info *MediaInfo) {
	for _, stream := range output.Streams {
		parseStream(stream, info)
	}
}

// parseStream parses a single stream from the probe output.
func parseStream(stream probeStream, info *MediaInfo) {
	switch stream.CodecType {
	case "video":
		if info.VideoCodec == emptyVideoCodec {
			info.VideoCodec = stream.CodecName
			info.Width = stream.Width
			info.Height = stream.Height
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
func audioTitle(tags probeTags) string {
	if tags.Title != "" {
		return tags.Title
	}

	return tags.Name
}
