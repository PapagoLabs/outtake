// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

func TestNewFromConfig_SQLite(t *testing.T) {
	t.Parallel()

	db, err := NewFromConfig(testDatabaseConfig(t.TempDir()+"/app.db", "sqlite", ""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Conn().PingContext(t.Context()))
}

func TestNewFromConfig_UnknownBackend(t *testing.T) {
	t.Parallel()

	_, err := NewFromConfig(testDatabaseConfig(t.TempDir()+"/app.db", "mysql", ""))
	require.ErrorIs(t, err, errUnknownDatabaseBackend)
}

func TestNewFromConfig_PostgresRequiresURL(t *testing.T) {
	t.Parallel()

	_, err := NewFromConfig(testDatabaseConfig("", "postgres", ""))
	require.ErrorIs(t, err, errDatabaseURLRequired)
}

func TestNewFromConfig_PgxAlias(t *testing.T) {
	t.Parallel()

	_, err := NewFromConfig(testDatabaseConfig("", "pgx", ""))
	require.ErrorIs(t, err, errDatabaseURLRequired)
}

func TestPostgres_SkipWithoutURL(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("OUTTAKE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("OUTTAKE_TEST_DATABASE_URL not set")
	}

	db, err := NewFromConfig(testDatabaseConfig("", "postgres", dsn))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	clipID := t.Name() + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Cleanup(func() { _ = db.DeleteClip(context.WithoutCancel(t.Context()), clipID) })

	job := &clip.Job{
		ID:         clipID,
		Type:       clip.TypeClip,
		Name:       "Intro",
		MediaID:    "100",
		MediaTitle: "Test Movie",
		MediaType:  "movie",

		StartTime:     10 * time.Second,
		Duration:      15 * time.Second,
		Quality:       "medium",
		Width:         0,
		FPS:           0,
		AudioIndex:    1,
		CropBlackBars: true,

		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second), InputPath: "/media/movie.mkv",
		OutputPath: "/out/clip-1.mp4",

		Status:   clip.StatusPending,
		Progress: 0,
		Error:    "",
	}

	require.NoError(t, db.SaveClip(t.Context(), job))

	got, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.MediaTitle, got.MediaTitle)
}

func testDatabaseConfig(path, backend, url string) *config.Config {
	return &config.Config{
		ListenAddr:      "",
		DatabasePath:    path,
		DatabaseBackend: backend,
		DatabaseURL:     url,
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
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   "",
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}
}
