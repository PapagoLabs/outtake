// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/queue"
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
	dsn := os.Getenv("OUTTAKE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("OUTTAKE_TEST_DATABASE_URL not set")
	}

	db, err := NewFromConfig(testDatabaseConfig("", "postgres", dsn))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := &queue.Job{
		ID:            "pg-clip-1",
		Type:          queue.JobTypeClip,
		Name:          "Intro",
		MediaID:       "100",
		MediaTitle:    "Test Movie",
		MediaType:     "movie",
		InputPath:     "/media/movie.mkv",
		OutputPath:    "/out/clip-1.mp4",
		StartTime:     10,
		Duration:      15,
		Quality:       "medium",
		Width:         0,
		FPS:           0,
		AudioIndex:    1,
		CropBlackBars: true,
		Status:        queue.JobStatusPending,
		Progress:      0,
		Error:         "",
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	}

	require.NoError(t, db.SaveClip(t.Context(), job))
	got, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.MediaTitle, got.MediaTitle)
	require.NoError(t, db.DeleteClip(t.Context(), job.ID))
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
		SessionPollSec:  0,
		NumWorkers:      0,
		MaxClipDurSec:   0,
		CropBlackBars:   false,
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   "",
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}
}
