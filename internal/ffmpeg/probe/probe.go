// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/PapagoLabs/outtake/internal/timecode"
)

// Info represents media file information.
type Info struct {
	Duration      time.Duration `json:"duration"`
	Width         int           `json:"width"`
	Height        int           `json:"height"`
	VideoCodec    string        `json:"video_codec"`
	AudioCodec    string        `json:"audio_codec"`
	Format        string        `json:"format"`
	BitRate       int64         `json:"bit_rate"`
	ColorTransfer string        `json:"color_transfer"`
	AudioTracks   []Track       `json:"audio_tracks"`
}

// Track is one audio stream on a source file.
type Track struct {
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
	// probeTimeout is the probe timeout.
	probeTimeout = 30 * time.Second
	// bitRateBits is the bit size for integer parsing.
	bitRateBits = 64
	// decimalBase is the base 10 for integer parsing.
	decimalBase = 10
	// emptyVideoCodec is an empty video codec placeholder.
	emptyVideoCodec = ""
	// emptyAudioCodec is an empty audio codec placeholder.
	emptyAudioCodec = ""
)

// Probe reads a media file for information.
//
// Parameters:
//   - ctx: Cancellation and deadline for the probe.
//   - ffprobePath: Path to the ffprobe binary.
//   - path: Media file path.
//
// Returns:
//   - info: The probed media information.
//   - err: Non-nil when the file cannot be probed.
func Probe(ctx context.Context, ffprobePath, path string) (Info, error) {
	cleanPath := filepath.Clean(path)

	cacheable := true

	identity, ok := KeyFor(cleanPath)
	if ok {
		if cached, found := probeResults.get(identity); found {
			return cached, nil
		}
	} else {
		cacheable = false
	}

	args := []string{
		ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		cleanPath,
	}

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(probeCtx, args[0], args[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return Info{}, fmt.Errorf("probe: %w", err)
	}

	result, parseErr := parseProbeOutput(output)
	if parseErr != nil {
		return Info{}, fmt.Errorf("probe: %w", parseErr)
	}

	// A file that changed while it was being probed describes neither the
	// identity that was recorded before nor the file on disk now, so its result
	// is returned to the caller but never cached.
	if cacheable && identityUnchanged(cleanPath, identity) {
		probeResults.put(identity, result)
	}

	return result, nil
}

// parseProbeOutput parses the ffprobe output.
//
// Parameters:
//   - data: Raw ffprobe JSON bytes.
//
// Returns:
//   - info: The parsed media information.
//   - err: Non-nil when the payload is not valid ffprobe JSON.
func parseProbeOutput(data []byte) (Info, error) {
	var output probeOutput

	err := json.Unmarshal(data, &output)
	if err != nil {
		return Info{}, fmt.Errorf("parse probe output: %w", err)
	}

	info := Info{
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
//
// Parameters:
//   - output: Decoded ffprobe envelope.
//   - info: Result to fill in place.
func parseDuration(output probeOutput, info *Info) {
	if output.Format.Duration == "" {
		return
	}

	dur, err := timecode.Parse(output.Format.Duration)
	if err == nil {
		info.Duration = dur.Duration()
	}
}

// parseBitRate parses the bit rate from the probe output.
//
// Parameters:
//   - output: Decoded ffprobe envelope.
//   - info: Result to fill in place.
func parseBitRate(output probeOutput, info *Info) {
	if output.Format.BitRate == "" {
		return
	}

	br, err := strconv.ParseInt(output.Format.BitRate, decimalBase, bitRateBits)
	if err == nil {
		info.BitRate = br
	}
}

// parseStreams parses the streams from the probe output.
//
// Parameters:
//   - output: Decoded ffprobe envelope.
//   - info: Result to fill in place.
func parseStreams(output probeOutput, info *Info) {
	for index := range output.Streams {
		parseStream(output.Streams[index], info)
	}
}

// parseStream parses a single stream from the probe output.
//
// Parameters:
//   - stream: One ffprobe stream row.
//   - info: Result to fill in place.
func parseStream(stream probeStream, info *Info) {
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

		info.AudioTracks = append(info.AudioTracks, Track{
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
//   - tags: Optional ffprobe stream tags.
//
// Returns:
//   - title: The stream title, or the name tag when there is no title.
func audioTitle(tags probeTags) string {
	if tags.Title != "" {
		return tags.Title
	}

	return tags.Name
}
