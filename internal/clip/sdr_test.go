// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSDRPathSitsBesideTheClipFile covers where a video clip's SDR version is
// stored: beside the clip's MP4, and nowhere for a GIF, a screenshot, or a
// clip with no output yet.
func TestSDRPathSitsBesideTheClipFile(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/data/clips/abc.sdr.mp4", SDRPathFor("/data/clips/abc.mp4"))
	assert.Empty(t, SDRPathFor("/data/gifs/abc.gif"))
	assert.Empty(t, SDRPathFor(""))

	video := Job{Type: TypeClip, OutputPath: "/data/clips/abc.mp4"}
	assert.Equal(t, "/data/clips/abc.sdr.mp4", video.SDRPath())

	gif := Job{Type: TypeGIF, OutputPath: "/data/gifs/abc.gif"}
	assert.Empty(t, gif.SDRPath())
}

// TestMayHaveSDRVersionNeedsAVideoClipThatKeepsHDR covers which clips are
// asked about an SDR version, so a card costs no extra lookup otherwise.
func TestMayHaveSDRVersionNeedsAVideoClipThatKeepsHDR(t *testing.T) {
	t.Parallel()

	keeps := &Job{ID: "a", Type: TypeClip, PreserveHDR: true, OutputPath: "/c/a.mp4"}
	converts := &Job{ID: "b", Type: TypeClip, OutputPath: "/c/b.mp4"}
	gif := &Job{ID: "c", Type: TypeGIF, PreserveHDR: true, OutputPath: "/g/c.gif"}
	unrendered := &Job{ID: "d", Type: TypeClip, PreserveHDR: true}

	assert.True(t, keeps.MayHaveSDRVersion())
	assert.False(t, converts.MayHaveSDRVersion())
	assert.False(t, gif.MayHaveSDRVersion())
	assert.False(t, unrendered.MayHaveSDRVersion())

	assert.Equal(t, []string{"/c/a.sdr.mp4"}, SDRPaths([]*Job{keeps, converts, gif, unrendered}))
}
