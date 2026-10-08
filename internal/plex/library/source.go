// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// AudioStream is one audio track on a source file.
type AudioStream struct {
	// Index is the ffmpeg stream index the export posts back.
	Index int
	// Codec is the audio codec name ffprobe reported.
	Codec string
	// Language is the stream language, which may be undetermined.
	Language string
	// Title is the stream title, which is often empty.
	Title string
	// Channels is the channel count ffprobe reported.
	Channels int
	// Layout is the channel layout name, empty when the count is unknown.
	Layout string
}

// SourceInfo is what probing a media file turned up.
type SourceInfo struct {
	// Path is the local file the source resolved to.
	Path string
	// Duration is how long the source runs, zero when it could not be probed.
	Duration time.Duration
	// ColorTransfer is the video color transfer ffprobe reported.
	ColorTransfer string
	// HDR reports whether the video transfer is HDR.
	HDR bool
	// Quality is the recommended encode quality for the source.
	Quality string
	// AudioStreams are the audio tracks the source carries.
	AudioStreams []AudioStream
}

// Prober reports what a media file holds.
type Prober interface {
	// Probe returns what ffprobe found in a media file.
	Probe(ctx context.Context, path string) (probe.Info, error)
}

// MediaSource resolves a Plex media item to a local file and describes it.
type MediaSource struct {
	prober   Prober
	cfg      *config.Config
	selected ServerSelection
}

// NewMediaSource returns a source resolver bound to the selected Plex server.
//
// Parameters:
//   - cfg: Application configuration.
//   - selected: Plex server selection to resolve against.
//   - prober: Runner that reports what a media file holds.
//
// Returns:
//   - source: The resolver under construction.
func NewMediaSource(
	cfg *config.Config,
	selected ServerSelection,
	prober Prober,
) *MediaSource {
	return &MediaSource{
		prober:   prober,
		cfg:      cfg,
		selected: selected,
	}
}

// CheckEdit probes a source once and reports whether an edit fits it: within
// the length cap, inside the source, and naming an audio track it carries.
//
// Parameters:
//   - ctx: Request context.
//   - path: Local media file path.
//   - edit: The change, with its type resolved.
//   - limit: Longest clip the installation accepts.
//
// Returns:
//   - err: Non-nil when the edit may not be rendered from this source.
func (source *MediaSource) CheckEdit(
	ctx context.Context,
	path string,
	edit clip.Edit,
	limit time.Duration,
) error {
	facts := clip.Source{Length: 0, AudioTracks: 0, Probed: false}

	info, err := source.prober.Probe(ctx, path)
	if err == nil {
		facts = clip.Source{
			Length:      max(info.Duration, 0),
			AudioTracks: len(info.AudioTracks),
			Probed:      true,
		}
	}

	err = edit.Validate(edit.Type, facts, limit)
	if err != nil {
		return fmt.Errorf("check edit: %w", err)
	}

	return nil
}

// Describe resolves a Plex media item and probes the file behind it.
//
// Parameters:
//   - ctx: Request context.
//   - mediaID: Plex rating key of the source item.
//
// Returns:
//   - info: What the source turned out to be, zero when it could not be
//     resolved or probed.
func (source *MediaSource) Describe(ctx context.Context, mediaID string) SourceInfo {
	if mediaID == "" {
		return SourceInfo{}
	}

	path, err := source.Resolve(ctx, mediaID)
	if err != nil {
		return SourceInfo{}
	}

	return source.probeFile(ctx, path)
}

// DescribePath probes a media file that has already been resolved.
//
// Parameters:
//   - ctx: Request context.
//   - path: Local media file path.
//
// Returns:
//   - info: What the file turned out to be, zero when it could not be probed.
func (source *MediaSource) DescribePath(ctx context.Context, path string) SourceInfo {
	return source.probeFile(ctx, path)
}

// DescribePaths probes several media files at once, at most limit at a time,
// and returns what each turned out to be. An empty or repeated path is probed
// once at most, and a probe that fails reads as a zero description.
//
// Parameters:
//   - ctx: Request context.
//   - paths: Local media file paths, which may repeat.
//   - limit: How many probes may run at once, at least one.
//
// Returns:
//   - infos: Each distinct path's description.
func (source *MediaSource) DescribePaths(
	ctx context.Context,
	paths []string,
	limit int,
) map[string]SourceInfo {
	var (
		mu    sync.Mutex
		group sync.WaitGroup
	)

	infos := make(map[string]SourceInfo, len(paths))
	slots := make(chan struct{}, max(limit, 1))

	for _, path := range slices.Compact(slices.Sorted(slices.Values(paths))) {
		if path == "" {
			continue
		}

		slots <- struct{}{}

		group.Go(func() {
			defer func() { <-slots }()

			info := source.probeFile(ctx, path)

			mu.Lock()

			infos[path] = info
			mu.Unlock()
		})
	}

	group.Wait()

	return infos
}

// Duration probes a media file for its length.
//
// Parameters:
//   - ctx: Request context.
//   - path: Local media file path.
//
// Returns:
//   - duration: How long the file runs.
//   - ok: False when the probe could not run or reported no length.
func (source *MediaSource) Duration(ctx context.Context, path string) (time.Duration, bool) {
	info, err := source.prober.Probe(ctx, path)
	if err != nil || info.Duration <= 0 {
		return 0, false
	}

	return info.Duration, true
}

// Resolve maps a Plex media item onto a local filesystem path.
//
// Parameters:
//   - ctx: Request context.
//   - mediaID: Plex rating key of the source item.
//
// Returns:
//   - path: Remapped local path for the media file.
//   - err: Non-nil when no server is selected or the lookup fails.
func (source *MediaSource) Resolve(ctx context.Context, mediaID string) (string, error) {
	path, err := ResolveMediaPath(ctx, source.cfg, source.selected, mediaID)
	if err != nil {
		return "", fmt.Errorf("resolve media path: %w", err)
	}

	return path, nil
}

// probeFile probes a file and turns the probe into a source description.
//
// Parameters:
//   - ctx: Request context.
//   - path: Local media file path.
//
// Returns:
//   - info: What the file turned out to be, zero when it could not be probed.
func (source *MediaSource) probeFile(ctx context.Context, path string) SourceInfo {
	info, err := source.prober.Probe(ctx, path)
	if err != nil {
		return SourceInfo{Path: path}
	}

	return newSourceInfo(path, info)
}

// newSourceInfo turns a probe result into a source description.
//
// Parameters:
//   - path: Local media file path the probe ran against.
//   - info: Probe result.
//
// Returns:
//   - source: The described source.
func newSourceInfo(path string, info probe.Info) SourceInfo {
	streams := make([]AudioStream, 0, len(info.AudioTracks))

	for _, track := range info.AudioTracks {
		streams = append(streams, AudioStream{
			Index:    track.Index,
			Codec:    track.Codec,
			Language: track.Language,
			Title:    track.Title,
			Channels: track.Channels,
			Layout:   ffmpeg.ChannelLayoutName(track.Channels),
		})
	}

	return SourceInfo{
		Path:          path,
		Duration:      info.Duration,
		ColorTransfer: info.ColorTransfer,
		HDR:           ffmpeg.IsHDRSource(info.ColorTransfer),
		Quality:       ffmpeg.SourceQuality(info),
		AudioStreams:  streams,
	}
}
