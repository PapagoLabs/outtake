// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
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

func TestLoad_EnvMaxClipDurIsSeconds(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OUTTAKE_MAX_CLIP_DUR", "90")

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, cfg.MaxClipDur)
}

func TestLoad_RunsAtLeastOneWorker(t *testing.T) {
	for _, workers := range []string{"0", "-3"} {
		t.Run(workers, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("OUTTAKE_NUM_WORKERS", workers)

			cfg, err := Load("")
			require.NoError(t, err)
			assert.Equal(t, 1, cfg.NumWorkers, "a queue with no workers would never render a clip")
		})
	}
}

// TestLoad_FFmpegTimeoutIsSeconds covers OUTTAKE_FFMPEG_TIMEOUT_SEC, a second
// count whose zero default scales the limit with each clip.
func TestLoad_FFmpegTimeoutIsSeconds(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Zero(t, cfg.FFmpegTimeout, "the default leaves the limit to scale with the clip")

	t.Setenv("OUTTAKE_FFMPEG_TIMEOUT_SEC", "5400")

	cfg, err = Load("")
	require.NoError(t, err)
	assert.Equal(t, 90*time.Minute, cfg.FFmpegTimeout)
}

func TestLoad_DefaultMaxClipDur(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OUTTAKE_MAX_CLIP_DUR", "")

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, clip.MaxDuration, cfg.MaxClipDur)
	assert.Equal(t, 10*time.Minute, cfg.MaxClipDur)
	assert.Positive(t, cfg.MaxClipDur)
}

func TestSecondsToDurationHook(t *testing.T) {
	t.Parallel()

	durationType := reflect.TypeFor[time.Duration]()
	int64Source := reflect.TypeFor[int64]()
	intSource := reflect.TypeFor[int]()
	stringSource := reflect.TypeFor[string]()
	stringTarget := reflect.TypeFor[string]()

	fromInt64, err := secondsToDurationHook(int64Source, durationType, int64(90))
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, fromInt64)

	fromInt, err := secondsToDurationHook(intSource, durationType, 90)
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, fromInt)

	fromString, err := secondsToDurationHook(stringSource, durationType, "90")
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, fromString,
		"an environment value is a string, and it is still a count of seconds")

	toString, err := secondsToDurationHook(int64Source, stringTarget, int64(90))
	require.NoError(t, err)
	assert.Equal(t, int64(90), toString)

	zero, err := secondsToDurationHook(int64Source, durationType, int64(0))
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), zero)

	negative, err := secondsToDurationHook(int64Source, durationType, int64(-5))
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), negative,
		"a negative seconds value clamps to zero rather than becoming a negative duration")

	overflowing, err := secondsToDurationHook(int64Source, durationType, int64(clip.MaxDuration))
	require.NoError(t, err)
	assert.NotEqual(t, time.Duration(clip.MaxDuration), overflowing)
}

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
		SessionPoll:     0,
		NumWorkers:      0,
		MaxClipDur:      0,
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
	assert.Equal(t, "/data/media-other/a.mkv", cfg.RemapMediaPath("/data/media-other/a.mkv"),
		"a root of /data/media does not claim /data/media-other")
	assert.Empty(t, cfg.RemapMediaPath("/data/media/../../etc/passwd"),
		"a .. segment must not leave the local mount")

	localOnly := testConfig()

	localOnly.LocalMediaRoot = "/media"
	assert.Equal(t, "/media/Movies/Example.mkv", localOnly.RemapMediaPath("/Movies/Example.mkv"))
	assert.Empty(t, localOnly.RemapMediaPath("/Movies/../../etc/passwd"))

	empty := testConfig()
	assert.Equal(t, "/data/media/a.mkv", empty.RemapMediaPath("/data/media/a.mkv"))
}

// TestRemapMediaPathMapsWindowsPaths covers a Plex server on Windows: drive
// letter and UNC paths, backslashes, a root matched without regard to case,
// and traversal still refused.
func TestRemapMediaPathMapsWindowsPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plexRoot  string
		plexPath  string
		wantLocal string
	}{
		{"a drive path under the root", `D:\Media`, `D:\Media\Movies\a.mkv`, "/media/Movies/a.mkv"},
		{"a root written with slashes", "D:/Media", `D:\Media\Movies\a.mkv`, "/media/Movies/a.mkv"},
		{"a root in another case", `d:\media`, `D:\Media\Movies\a.mkv`, "/media/Movies/a.mkv"},
		{"a UNC share", `\\nas\share\Media`, `\\NAS\share\Media\Shows\b.mkv`, "/media/Shows/b.mkv"},
		{"the root itself", `D:\Media`, `D:\Media`, "/media"},
		{"a sibling of the root", `D:\Media`, `D:\Media-other\a.mkv`, `D:\Media-other\a.mkv`},
		{"another drive", `D:\Media`, `E:\Media\a.mkv`, `E:\Media\a.mkv`},
		{"a traversal out of the mount", `D:\Media`, `D:\Media\..\..\etc\passwd`, ""},
		{"a drive path with no root", "", `D:\Media\Movies\a.mkv`, "/media/Media/Movies/a.mkv"},
		{"a UNC path with no root", "", `\\nas\share\Movies\a.mkv`, "/media/Movies/a.mkv"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := testConfig()

			cfg.PlexMediaRoot = test.plexRoot
			cfg.LocalMediaRoot = "/media"

			assert.Equal(t, test.wantLocal, cfg.RemapMediaPath(test.plexPath))
		})
	}

	linux := testConfig()

	linux.PlexMediaRoot = "/data/media"
	linux.LocalMediaRoot = "/media"
	assert.Equal(t, `/media/odd\name.mkv`, linux.RemapMediaPath(`/data/media/odd\name.mkv`),
		"a backslash in a Linux file name is part of the name")

	linux.PlexMediaRoot = ""
	assert.Equal(t, "/media/data/x.mkv", linux.RemapMediaPath("//data/x.mkv"),
		"a Linux path that starts with two slashes is not a UNC share")
}
