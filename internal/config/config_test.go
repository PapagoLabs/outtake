// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:8080", cfg.ListenAddr)
	assert.Contains(t, cfg.DatabasePath, "outtake/outtake.db")
	assert.Contains(t, cfg.StoragePath, "outtake/output")
	assert.Equal(t, "sqlite", cfg.DatabaseBackend)
	assert.Empty(t, cfg.DatabaseURL)
	assert.Equal(t, "filesystem", cfg.StorageBackend)
	assert.Equal(t, "us-east-1", cfg.S3Region)
	assert.True(t, cfg.S3UsePathStyle)
}

func TestLoad_LocalMediaRootEnv(t *testing.T) {
	t.Setenv("OUTTAKE_LOCAL_MEDIA_ROOT", "/media")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "/media", cfg.LocalMediaRoot)
	assert.Equal(t, "/media/Movies/Example.mkv", cfg.RemapMediaPath("/Movies/Example.mkv"))
}

func TestLoad_StorageAndDatabaseBackendEnv(t *testing.T) {
	t.Setenv("OUTTAKE_STORAGE_BACKEND", "s3")
	t.Setenv("OUTTAKE_S3_ENDPOINT", "http://localhost:8333")
	t.Setenv("OUTTAKE_S3_BUCKET", "outtake")
	t.Setenv("OUTTAKE_S3_REGION", "us-west-2")
	t.Setenv("OUTTAKE_S3_ACCESS_KEY", "key")
	t.Setenv("OUTTAKE_S3_SECRET_KEY", "secret")
	t.Setenv("OUTTAKE_S3_USE_PATH_STYLE", "true")
	t.Setenv("OUTTAKE_DATABASE_BACKEND", "postgres")
	t.Setenv("OUTTAKE_DATABASE_URL", "postgres://outtake@localhost:5432/outtake")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "s3", cfg.StorageBackend)
	assert.Equal(t, "http://localhost:8333", cfg.S3Endpoint)
	assert.Equal(t, "outtake", cfg.S3Bucket)
	assert.Equal(t, "us-west-2", cfg.S3Region)
	assert.Equal(t, "key", cfg.S3AccessKey)
	assert.Equal(t, "secret", cfg.S3SecretKey)
	assert.True(t, cfg.S3UsePathStyle)
	assert.Equal(t, "postgres", cfg.DatabaseBackend)
	assert.Equal(t, "postgres://outtake@localhost:5432/outtake", cfg.DatabaseURL)
}

func TestLoad_CustomConfigFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configFile := filepath.Join(dir, "custom.yaml")

	cfg, err := Load(configFile)
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:8080", cfg.ListenAddr)
}

func TestLoad_CreatesDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_ = dir

	cfg, err := Load("")
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.DatabasePath)
	assert.NotEmpty(t, cfg.StoragePath)
}

func TestLoad_XDGPaths(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Contains(t, cfg.DatabasePath, "outtake/outtake.db")
	assert.Contains(t, cfg.StoragePath, "outtake/output")
}

// testConfig returns the test config.
//
// Returns:
//   - cfg: The test config.
func testConfig() Config {
	return Config{
		ListenAddr:      "",
		DatabasePath:    "",
		DatabaseBackend: "",
		DatabaseURL:     "",
		StoragePath:     "",
		StorageBackend:  "",
		S3Endpoint:      "",
		S3Bucket:        "",
		S3Region:        "",
		S3AccessKey:     "",
		S3SecretKey:     "",
		S3UsePathStyle:  false,
		FFmpegPath:      "",
		FFprobePath:     "",
		LogLevel:        "",
		Env:             "",
		SessionPollSec:  0,
		NumWorkers:      0,
		MaxClipDurSec:   0,
		CropBlackBars:   false,
		WebSafeColor:    false,
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   "",
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}
}

func TestPublicURL(t *testing.T) {
	t.Parallel()

	listenAll := testConfig()

	listenAll.ListenAddr = "0.0.0.0:8080"
	assert.Equal(t, "http://localhost:8080", listenAll.PublicURL())

	listenPort := testConfig()

	listenPort.ListenAddr = ":9090"
	assert.Equal(t, "http://localhost:9090", listenPort.PublicURL())

	public := testConfig()

	public.PublicBaseURL = "https://clips.example/"
	assert.Equal(t, "https://clips.example", public.PublicURL())
}

func TestRemapMediaPath(t *testing.T) {
	t.Parallel()

	cfg := testConfig()

	cfg.PlexMediaRoot = "/data/media"
	cfg.LocalMediaRoot = "/media"

	assert.Equal(t, "/media/movies/a.mkv", cfg.RemapMediaPath("/data/media/movies/a.mkv"))
	assert.Equal(t, "/other/a.mkv", cfg.RemapMediaPath("/other/a.mkv"))

	localOnly := testConfig()

	localOnly.LocalMediaRoot = "/media"
	assert.Equal(t, "/media/Movies/Example.mkv", localOnly.RemapMediaPath("/Movies/Example.mkv"))

	empty := testConfig()
	assert.Equal(t, "/data/media/a.mkv", empty.RemapMediaPath("/data/media/a.mkv"))
}
