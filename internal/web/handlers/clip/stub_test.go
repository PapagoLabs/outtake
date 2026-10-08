// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// stubProber reports a fixed length for any media file.
type stubProber struct {
	duration time.Duration
	// transfer is the video color transfer the probe reports, empty for SDR.
	transfer string
}

// Probe returns a length for any path.
//
// Parameters:
//   - ctx: Request context, unused.
//   - path: Media file path, unused.
//
// Returns:
//   - info: A probe result carrying the stubbed length and three audio tracks.
//   - err: Always nil.
func (prober *stubProber) Probe(_ context.Context, _ string) (probe.Info, error) {
	// Three audio tracks, so a test may pick any track up to the third.
	tracks := []probe.Track{{Index: 0}, {Index: 1}, {Index: 2}}

	return probe.Info{
		Duration:      prober.duration,
		AudioTracks:   tracks,
		ColorTransfer: prober.transfer,
	}, nil
}

// noopJobHandler is a queue worker that renders nothing, for tests that only
// care what the queue does with a job rather than what the render produces.
func noopJobHandler(_ context.Context, _ *clipdom.Job) error {
	return nil
}

// testClipJob is a completed clip for tests that overwrite the fields they care
// about and leave the rest alone.
//
// Parameters:
//   - id: Clip id.
//   - kind: Type of clip.
//
// Returns:
//   - job: The clip under test.
func testClipJob(id string, kind clipdom.Type) *clipdom.Job {
	return &clipdom.Job{
		ID:         id,
		Type:       kind,
		Name:       "Intro",
		MediaID:    "42",
		MediaTitle: "Movie",
		MediaType:  clipdom.DefaultMediaType,

		StartTime:  0,
		Duration:   0,
		Quality:    string(clipdom.ClipQualityMedium),
		AudioIndex: 0,

		CreatedAt: time.Time{},
		UpdatedAt: time.Time{}, InputPath: "/media/test.mkv",

		Status: clipdom.StatusCompleted,
	}
}

// stubSources returns a media source resolver whose probe always reports the
// given length.
//
// Parameters:
//   - t: The test the resolver belongs to.
//   - duration: Length every probed source reports.
//
// Returns:
//   - sources: A resolver bound to no Plex server.
func stubSources(t *testing.T, duration time.Duration) *library.MediaSource {
	t.Helper()

	return library.NewMediaSource(
		&config.Config{MaxClipDur: 10 * time.Minute},
		nil,
		&stubProber{duration: duration},
	)
}

// closeBody closes a response body and fails the test when it cannot.
//
// Parameters:
//   - t: The test the response belongs to.
//   - resp: The response to close.
func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()

	require.NoError(t, resp.Body.Close())
}

// hdrSources returns a media source resolver whose probe reports a two-hour
// PQ source with three audio tracks.
//
// Parameters:
//   - t: The test the resolver belongs to.
//
// Returns:
//   - sources: A resolver bound to no Plex server.
func hdrSources(t *testing.T) *library.MediaSource {
	t.Helper()

	return library.NewMediaSource(
		&config.Config{MaxClipDur: 10 * time.Minute},
		nil,
		&stubProber{duration: 2 * time.Hour, transfer: "smpte2084"},
	)
}
