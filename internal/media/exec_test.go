// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecFFmpeg_Run_CommandNotFound(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("/nonexistent/ffmpeg", "/nonexistent/ffprobe")
	err := ff.run(t.Context(), 0, "/nonexistent/ffmpeg", "-version")
	assert.Error(t, err)
}

func TestExecFFmpeg_Run_Timeout(t *testing.T) {
	t.Parallel()

	_, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}

	ff := NewExecFFmpeg("sleep", "sleep")
	ff.SetTimeout(100 * time.Millisecond)

	err = ff.run(t.Context(), 0, "sleep", "10")
	assert.Error(t, err)
}

func TestExecFFmpeg_ExtractClip_Args(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("echo", "echo")
	ctx := t.Context()

	err := ff.ExtractClip(ctx, "/tmp/input.mp4", "/tmp/output.mp4", 10.5, 30.0, ClipQualityHigh)
	require.NoError(t, err)
}

func TestExecFFmpeg_ExtractScreenshot_Args(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("echo", "echo")
	ctx := t.Context()

	err := ff.ExtractScreenshot(ctx, "/tmp/input.mp4", "/tmp/screenshot.jpg", 120.0)
	require.NoError(t, err)
}
