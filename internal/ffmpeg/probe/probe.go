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
	"strings"
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
	// DolbyVision is the video stream's Dolby Vision configuration, the zero
	// value when the stream carries none.
	DolbyVision DolbyVision `json:"dolby_vision"`
	// MaxCLLNits is the video stream's MaxCLL in nits, 0 when it is unknown.
	MaxCLLNits float64 `json:"max_cll_nits"`
	// MasteringMaxNits is the mastering display's peak in nits, 0 when it is
	// unknown.
	MasteringMaxNits float64 `json:"mastering_max_nits"`
}

// DolbyVision is a stream's Dolby Vision configuration record.
type DolbyVision struct {
	// Present reports that the stream carries a configuration record.
	Present bool `json:"present"`
	// Profile is the Dolby Vision profile, such as 5, 7, or 8.
	Profile int `json:"profile"`
	// BaseLayerCompatibility is the base layer's signal compatibility id:
	// 0 for none, 1 for HDR10, 2 for SDR, 4 for HLG, and 6 for UHD Blu-ray.
	BaseLayerCompatibility int `json:"base_layer_compatibility"`
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
	Index         int             `json:"index"`
	CodecType     string          `json:"codec_type"`
	CodecName     string          `json:"codec_name"`
	Width         int             `json:"width"`
	Height        int             `json:"height"`
	Channels      int             `json:"channels"`
	ColorTransfer string          `json:"color_transfer"`
	Tags          probeTags       `json:"tags"`
	SideData      []probeSideData `json:"side_data_list"`
}

// probeSideData is one entry of a stream's side data list. Only the Dolby
// Vision configuration record's fields are read.
type probeSideData struct {
	Type                   string `json:"side_data_type"`
	MaxLuminance           string `json:"max_luminance"`
	DolbyVisionProfile     int    `json:"dv_profile"`
	BaseLayerCompatibility int    `json:"dv_bl_signal_compatibility_id"`
	MaxContent             int    `json:"max_content"`
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
	// dolbyVisionRecordType is ffprobe's name for the Dolby Vision
	// configuration record.
	dolbyVisionRecordType = "DOVI configuration record"
	// dolbyVisionProfile5 is the profile whose base layer is not displayable.
	dolbyVisionProfile5 = 5
	// contentLightType is ffprobe's name for the content light level record.
	contentLightType = "Content light level metadata"
	// masteringDisplayType is ffprobe's name for the mastering display record.
	masteringDisplayType = "Mastering display metadata"
	// rationalSeparator splits ffprobe's numerator/denominator values.
	rationalSeparator = "/"
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
			info.DolbyVision = dolbyVision(stream.SideData)
			info.MaxCLLNits, info.MasteringMaxNits = lightLevels(stream.SideData)
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

// NeedsDolbyVisionReshaping reports whether the video is Dolby Vision with no
// displayable base layer, such as profile 5. Its base layer is in Dolby's own
// color space and only looks right after the Dolby Vision reshaping, which
// ffmpeg's decoder does not apply.
//
// Returns:
//   - needs: True for profile 5, or for any base layer compatible with nothing.
func (info Info) NeedsDolbyVisionReshaping() bool {
	return info.DolbyVision.Present &&
		(info.DolbyVision.Profile == dolbyVisionProfile5 ||
			info.DolbyVision.BaseLayerCompatibility == 0)
}

// PeakNits reports the brightest the source's HDR10 metadata says it gets:
// MaxCLL when the stream carries it, otherwise the mastering display's peak.
//
// Returns:
//   - nits: The peak in nits.
//   - ok: False when the stream carries neither.
func (info Info) PeakNits() (float64, bool) {
	if info.MaxCLLNits > 0 {
		return info.MaxCLLNits, true
	}

	if info.MasteringMaxNits > 0 {
		return info.MasteringMaxNits, true
	}

	return 0, false
}

// dolbyVision reads the Dolby Vision configuration record from a stream's side
// data.
//
// Parameters:
//   - sideData: The stream's side data list.
//
// Returns:
//   - config: The record, or the zero value when the stream carries none.
func dolbyVision(sideData []probeSideData) DolbyVision {
	for _, entry := range sideData {
		if entry.Type == dolbyVisionRecordType {
			return DolbyVision{
				Present:                true,
				Profile:                entry.DolbyVisionProfile,
				BaseLayerCompatibility: entry.BaseLayerCompatibility,
			}
		}
	}

	return DolbyVision{Present: false, Profile: 0, BaseLayerCompatibility: 0}
}

// lightLevels reads a stream's HDR10 light levels from its side data. A
// MaxCLL of 0 means the encoder did not know it, so it reads as unknown.
//
// Parameters:
//   - sideData: The stream's side data list.
//
// Returns:
//   - maxCLL: MaxCLL in nits, 0 when unknown.
//   - masteringMax: The mastering display's peak in nits, 0 when unknown.
//
//nolint:nonamedreturns // Same-type returns need names.
func lightLevels(sideData []probeSideData) (maxCLL, masteringMax float64) {
	for _, entry := range sideData {
		switch entry.Type {
		case contentLightType:
			maxCLL = float64(max(entry.MaxContent, 0))
		case masteringDisplayType:
			masteringMax = parseRational(entry.MaxLuminance)
		default:
		}
	}

	return maxCLL, masteringMax
}

// parseRational reads ffprobe's rational value, such as "10000000/10000".
//
// Parameters:
//   - raw: The value as ffprobe prints it.
//
// Returns:
//   - value: The value, or 0 when it is missing or malformed.
func parseRational(raw string) float64 {
	numerator, denominator, found := strings.Cut(raw, rationalSeparator)
	if !found {
		denominator = "1"
	}

	num, numErr := strconv.ParseFloat(numerator, bitRateBits)
	den, denErr := strconv.ParseFloat(denominator, bitRateBits)

	if numErr != nil || denErr != nil || den <= 0 || num <= 0 {
		return 0
	}

	return num / den
}
