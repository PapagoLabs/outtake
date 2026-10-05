// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// stubProber returns a fixed probe result, or an error, for any path.
type stubProber struct {
	info probe.Info
	err  error
}

// Probe returns the stubbed result.
//
// Parameters:
//   - ctx: Request context, unused.
//   - path: Media file path, unused.
//
// Returns:
//   - info: The stubbed probe result.
//   - err: The stubbed failure.
func (prober *stubProber) Probe(_ context.Context, _ string) (probe.Info, error) {
	return prober.info, prober.err
}

func newSourceUnderTest(prober Prober) *MediaSource {
	return NewMediaSource(&config.Config{}, nil, prober)
}

func TestNewSourceInfoDescribesWhatTheProbeFound(t *testing.T) {
	t.Parallel()

	source := newSourceInfo("/media/movie.mkv", probe.Info{
		Duration:      90 * time.Second,
		ColorTransfer: "smpte2084",
		AudioTracks: []probe.Track{
			{Index: 0, Codec: "aac", Language: "eng", Title: "Commentary", Channels: 2},
			{Index: 1, Codec: "dts", Language: "und", Channels: 8},
		},
	})

	assert.Equal(t, "/media/movie.mkv", source.Path)
	assert.Equal(t, 90*time.Second, source.Duration)
	assert.Equal(t, "smpte2084", source.ColorTransfer)
	assert.True(t, source.HDR, "a PQ transfer is HDR")
	assert.NotEmpty(t, source.Quality, "the probe drives the recommended encode quality")

	require.Len(t, source.AudioStreams, 2)
	assert.Equal(t, AudioStream{
		Index:    0,
		Codec:    "aac",
		Language: "eng",
		Title:    "Commentary",
		Channels: 2,
		Layout:   "Stereo",
	}, source.AudioStreams[0])
	assert.Equal(t, "7.1", source.AudioStreams[1].Layout,
		"the layout name is resolved here, so the adapters need not know it")
}

func TestNewSourceInfoWithNoAudioTracks(t *testing.T) {
	t.Parallel()

	source := newSourceInfo("/media/movie.mkv", probe.Info{})

	assert.Empty(t, source.AudioStreams)
	assert.False(t, source.HDR, "no transfer is not HDR")
	assert.Zero(t, source.Duration)
}

func TestDescribePathReportsAFailedProbe(t *testing.T) {
	t.Parallel()

	source := newSourceUnderTest(&stubProber{err: assert.AnError})

	assert.Equal(t, SourceInfo{Path: "/media/movie.mkv"},
		source.DescribePath(t.Context(), "/media/movie.mkv"),
		"the path is still reported, so a page can name what it failed to read")
}

func TestDescribeWithNoMediaID(t *testing.T) {
	t.Parallel()

	source := newSourceUnderTest(&stubProber{info: probe.Info{Duration: time.Minute}})

	assert.Zero(t, source.Describe(t.Context(), ""),
		"no media id names no source, so the probe is not run")
}

func TestDurationReportsTheProbedLength(t *testing.T) {
	t.Parallel()

	source := newSourceUnderTest(&stubProber{info: probe.Info{Duration: 42 * time.Second}})

	duration, ok := source.Duration(t.Context(), "/media/movie.mkv")
	require.True(t, ok)
	assert.Equal(t, 42*time.Second, duration)
}

func TestDurationWithNothingToReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prober Prober
	}{
		{name: "a failed probe", prober: &stubProber{err: assert.AnError}},
		{name: "a probe reporting no length", prober: &stubProber{info: probe.Info{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := newSourceUnderTest(test.prober)

			duration, ok := source.Duration(t.Context(), "/media/movie.mkv")

			assert.False(t, ok, "an unknown length bounds nothing")
			assert.Zero(t, duration)
		})
	}
}
