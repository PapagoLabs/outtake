// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/config"
)

func TestNew_DefaultBackends(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{
		ListenAddr:      "127.0.0.1:0",
		DatabasePath:    filepath.Join(dir, "outtake.db"),
		DatabaseBackend: "sqlite",
		DatabaseURL:     "",
		StoragePath:     filepath.Join(dir, "output"),
		StorageBackend:  "filesystem",
		S3Endpoint:      "",
		S3Bucket:        "",
		S3Region:        "",
		S3AccessKey:     "",
		S3SecretKey:     "",
		S3UsePathStyle:  true,
		FFmpegPath:      "ffmpeg",
		FFprobePath:     "ffprobe",
		LogLevel:        "info",
		Env:             "test",
		SessionPollSec:  10,
		NumWorkers:      1,
		MaxClipDurSec:   600,
		CropBlackBars:   false,
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   "",
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}

	application, err := New(cfg)
	require.NoError(t, err)
	application.Close()
}
