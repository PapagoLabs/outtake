// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// releasedSchema are the migrations the first tagged releases shipped.
var releasedSchema = []string{"001_initial.sql", "002_web_safe_color.sql", "003_preserve_hdr.sql"}

// releasedDatabase writes a database as a release with migrations 001 to 003
// left it.
//
// It holds a clip on each old built-in profile, one on a custom profile, and
// a stored Plex token, the way that release recorded them.
//
// Parameters:
//   - t: The test the database belongs to.
//
// Returns:
//   - path: The database file.
func releasedDatabase(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "outtake.db")

	conn, err := sql.Open("libsql", "file:"+path+sqliteOptions)
	require.NoError(t, err)

	old := &DB{conn: conn, dialect: dialectSQLite}

	_, err = conn.ExecContext(t.Context(), `CREATE TABLE schema_migrations (name TEXT PRIMARY KEY)`)
	require.NoError(t, err)

	for _, name := range releasedSchema {
		require.NoError(t, old.applyOne(t.Context(), name))
	}

	_, err = conn.ExecContext(t.Context(), `
		INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width)
		VALUES ('tiny', 'Tiny', 30, 'veryfast', 96, 640);
		INSERT INTO plex_tokens (client_id, access_token) VALUES ('client', 'fake-token');
		INSERT INTO clips (id, media_id, media_title, input_path, output_path,
			start_time, duration, quality, status, web_safe_color)
		VALUES
			('on-low', '1', 'Movie', '/media/a.mkv', '/data/on-low.mp4', 1, 5, 'low', 'completed', 1),
			('on-medium', '1', 'Movie', '/media/a.mkv', '/data/on-medium.mp4', 1, 5, 'medium', 'completed', 1),
			('on-high', '1', 'Movie', '/media/a.mkv', '/data/on-high.mp4', 1, 5, 'high', 'completed', 0),
			('on-tiny', '1', 'Movie', '/media/a.mkv', '/data/on-tiny.mp4', 1, 5, 'tiny', 'failed', 1);
	`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	return path
}

// TestUpgradeFromAReleaseKeepsEveryClip covers opening a database a release
// with migrations 001 to 003 wrote.
//
// Every later migration applies in order. Each clip is kept on a profile that
// exists, the custom profile with it, and the stored token still names who
// may claim the installation. The clips belong to nobody until the owner
// claims the installation, and then all of them are the owner's.
func TestUpgradeFromAReleaseKeepsEveryClip(t *testing.T) {
	t.Parallel()

	db, err := New(releasedDatabase(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	clips, err := db.ListClips(t.Context())
	require.NoError(t, err)
	require.Len(t, clips, 4)

	var orphans int

	require.NoError(t, db.conn.QueryRowContext(t.Context(), `
		SELECT COUNT(*) FROM clips c LEFT JOIN clip_profiles p ON p.id = c.quality
		WHERE p.id IS NULL
	`).Scan(&orphans))
	assert.Zero(t, orphans, "every clip's profile exists")

	tiny, err := db.GetClip(t.Context(), "on-tiny")
	require.NoError(t, err)
	assert.Equal(t, "tiny", tiny.Quality, "a custom profile keeps its clips")

	high, err := db.GetClip(t.Context(), "on-high")
	require.NoError(t, err)
	assert.True(t, high.PreserveHDR, "a clip that kept HDR still says so")

	token, err := db.LegacyToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "fake-token", token)

	for _, job := range clips {
		assert.Zero(t, job.OwnerID, "%s belongs to nobody before the claim", job.ID)
	}

	claimed, err := db.ClaimOwner(t.Context(), 42, "owner")
	require.NoError(t, err)
	require.True(t, claimed)

	clips, err = db.ListClips(t.Context())
	require.NoError(t, err)

	for _, job := range clips {
		assert.Equal(t, 42, job.OwnerID, "%s is the owner's after the claim", job.ID)
	}
}

// TestApplySQLKeepsNothingOfAFailedMigration covers a migration whose second
// statement fails.
//
// The first statement is rolled back and the migration is not recorded, so
// the next start runs it again from the beginning.
func TestApplySQLKeepsNothingOfAFailedMigration(t *testing.T) {
	t.Parallel()

	db, err := New(filepath.Join(t.TempDir(), "outtake.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	err = db.applySQL(t.Context(), "999_broken.sql", `
		ALTER TABLE clips ADD COLUMN half_done INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE no_such_table ADD COLUMN never INTEGER;
	`)
	require.Error(t, err)

	var columns int

	require.NoError(t, db.conn.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM pragma_table_info('clips') WHERE name = 'half_done'`,
	).Scan(&columns))
	assert.Zero(t, columns, "the first statement was rolled back")

	applied, err := db.appliedMigrations(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, applied, "999_broken.sql")
}
