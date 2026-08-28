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

func testConfig() Config {
	return Config{
		ListenAddr:     "",
		DatabasePath:   "",
		StoragePath:    "",
		FFmpegPath:     "",
		FFprobePath:    "",
		LogLevel:       "",
		Env:            "",
		SessionPollSec: 0,
		NumWorkers:     0,
		MaxClipDurSec:  0,
		PlexServerURL:  "",
		PlexToken:      "",
		PlexClientID:   "",
		PublicBaseURL:  "",
		PlexMediaRoot:  "",
		LocalMediaRoot: "",
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
