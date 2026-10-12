// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip/quota"
)

// overrideColumns are a user's nullable limit columns as scanned.
type overrideColumns struct {
	// active is limit_active.
	active sql.NullInt64
	// weekly is limit_weekly.
	weekly sql.NullInt64
	// storage is limit_storage_bytes.
	storage sql.NullInt64
	// clipSeconds is limit_clip_seconds.
	clipSeconds sql.NullInt64
}

// UserUsage is what one user's renders and clips come to.
type UserUsage struct {
	// Bytes is the storage the user's clips take.
	Bytes int64
	// Renders is how many renders the user started in the span counted.
	Renders int
}

// Settings the global limits are stored under in app_settings.
const (
	// settingLimitActive is the global limit on concurrent renders.
	settingLimitActive = "limit_active"
	// settingLimitWeekly is the global limit on renders a week.
	settingLimitWeekly = "limit_weekly"
	// settingLimitStorage is the global storage limit in bytes.
	settingLimitStorage = "limit_storage_bytes"
	// settingLimitClipSeconds is the global longest clip in seconds.
	settingLimitClipSeconds = "limit_clip_seconds"
)

// Numbers a stored limit is written in.
const (
	// decimalBase is the base a limit is stored in.
	decimalBase = 10
	// int64Bits is the size a stored limit is read at.
	int64Bits = 64
)

// GlobalLimits returns the render limits every member has.
//
// A limit never saved takes its default.
//
// Parameters:
//   - ctx: Request scope for the reads.
//
// Returns:
//   - limits: The global limits.
//   - err: Non-nil when a setting cannot be read or is not a number.
func (db *DB) GlobalLimits(ctx context.Context) (quota.Limits, error) {
	limits := quota.Defaults()

	for _, setting := range []struct {
		name  string
		apply func(value int64)
	}{
		{name: settingLimitActive, apply: func(value int64) { limits.Active = int(value) }},
		{name: settingLimitWeekly, apply: func(value int64) { limits.Weekly = int(value) }},
		{name: settingLimitStorage, apply: func(value int64) { limits.Storage = value }},
		{
			name:  settingLimitClipSeconds,
			apply: func(value int64) { limits.ClipLength = time.Duration(value) * time.Second },
		},
	} {
		value, found, err := db.Setting(ctx, setting.name)
		if err != nil {
			return quota.Limits{}, fmt.Errorf("global limits: %w", err)
		}

		if !found {
			continue
		}

		number, err := strconv.ParseInt(value, decimalBase, int64Bits)
		if err != nil {
			return quota.Limits{}, fmt.Errorf("global limit %s: %w", setting.name, err)
		}

		setting.apply(number)
	}

	return limits, nil
}

// RecordRender logs a render a user started, for the rolling weekly limit.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - owner: The user's plex.tv id.
//   - at: When the render started.
//
// Returns:
//   - err: Non-nil when the write fails.
func (db *DB) RecordRender(ctx context.Context, owner int, at time.Time) error {
	_, err := db.conn.ExecContext(
		ctx,
		db.rewrite(`INSERT INTO render_log (owner_id, rendered_at) VALUES (?, ?)`),
		owner,
		at.Unix(),
	)
	if err != nil {
		return fmt.Errorf("record render: %w", err)
	}

	return nil
}

// SweepRenderLog deletes the render log rows older than a time.
//
// Only rows the weekly limit no longer counts are deleted, so the log keeps
// no more than one window of renders.
//
// Parameters:
//   - ctx: Request scope for the delete.
//   - before: The start of the span the limit counts.
//
// Returns:
//   - removed: How many rows were deleted.
//   - err: Non-nil when the delete fails.
func (db *DB) SweepRenderLog(ctx context.Context, before time.Time) (int64, error) {
	result, err := db.conn.ExecContext(
		ctx,
		db.rewrite(`DELETE FROM render_log WHERE rendered_at < ?`),
		before.Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("sweep render log: %w", err)
	}

	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sweep render log rows: %w", err)
	}

	return removed, nil
}

// RendersSince counts the renders a user started since a time.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - owner: The user's plex.tv id.
//   - since: The start of the span counted.
//
// Returns:
//   - started: How many renders started in the span.
//   - oldest: When the oldest of them started, zero when none did.
//   - err: Non-nil when the read fails.
func (db *DB) RendersSince(
	ctx context.Context,
	owner int,
	since time.Time,
) (int, time.Time, error) {
	var (
		started int
		oldest  sql.NullInt64
	)

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(
			`SELECT COUNT(*), MIN(rendered_at) FROM render_log WHERE owner_id = ? AND rendered_at >= ?`,
		),
		owner,
		since.Unix(),
	).Scan(&started, &oldest)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("renders since: %w", err)
	}

	if !oldest.Valid {
		return started, time.Time{}, nil
	}

	return started, time.Unix(oldest.Int64, 0), nil
}

// UsageByOwner returns what every user's renders and clips come to, in two
// grouped reads.
//
// Parameters:
//   - ctx: Request scope for the reads.
//   - since: The start of the span renders are counted over.
//
// Returns:
//   - usage: Each user's usage by plex.tv id, absent for a user with neither
//     renders in the span nor clips.
//   - err: Non-nil when a read fails.
func (db *DB) UsageByOwner(ctx context.Context, since time.Time) (map[int]UserUsage, error) {
	usage := make(map[int]UserUsage)

	err := db.groupByOwner(
		ctx,
		`SELECT owner_id, COUNT(*) FROM render_log WHERE rendered_at >= ? GROUP BY owner_id`,
		func(owner int, value int64) {
			entry := usage[owner]

			entry.Renders = int(value)
			usage[owner] = entry
		},
		since.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("count renders: %w", err)
	}

	err = db.groupByOwner(
		ctx,
		`SELECT owner_id, COALESCE(SUM(output_bytes), 0) FROM clips GROUP BY owner_id`,
		func(owner int, value int64) {
			entry := usage[owner]

			entry.Bytes = value
			usage[owner] = entry
		},
	)
	if err != nil {
		return nil, fmt.Errorf("sum storage: %w", err)
	}

	return usage, nil
}

// groupByOwner reads a query that yields one owner and one number per row.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - query: The query, with ? placeholders.
//   - record: Takes each row's owner and number.
//   - args: The placeholders' values.
//
// Returns:
//   - err: Non-nil when the read fails.
func (db *DB) groupByOwner(
	ctx context.Context,
	query string,
	record func(owner int, value int64),
	args ...any,
) error {
	rows, err := db.conn.QueryContext(ctx, db.rewrite(query), args...)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			owner int
			value int64
		)

		err = rows.Scan(&owner, &value)
		if err != nil {
			return fmt.Errorf("scan: %w", err)
		}

		record(owner, value)
	}

	err = rows.Err()
	if err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	return nil
}

// SaveGlobalLimits stores the render limits every member has.
//
// Parameters:
//   - ctx: Request scope for the writes.
//   - limits: The global limits, where zero sets no limit.
//
// Returns:
//   - err: Non-nil when a setting cannot be written.
func (db *DB) SaveGlobalLimits(ctx context.Context, limits quota.Limits) error {
	for name, value := range map[string]int64{
		settingLimitActive:      int64(limits.Active),
		settingLimitWeekly:      int64(limits.Weekly),
		settingLimitStorage:     limits.Storage,
		settingLimitClipSeconds: int64(limits.ClipLength / time.Second),
	} {
		err := db.SaveSetting(ctx, name, strconv.FormatInt(value, decimalBase))
		if err != nil {
			return fmt.Errorf("save global limits: %w", err)
		}
	}

	return nil
}

// SaveUserLimits stores one user's own render limits.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - plexUserID: The user, other than the owner.
//   - override: The user's limits, where nil takes the global value.
//
// Returns:
//   - err: ErrUserNotFound when no user other than the owner is stored under
//     the id, otherwise the write failure.
func (db *DB) SaveUserLimits(ctx context.Context, plexUserID int, override quota.Override) error {
	var clipSeconds *int64

	if override.ClipLength != nil {
		seconds := int64(*override.ClipLength / time.Second)

		clipSeconds = &seconds
	}

	err := db.changeUser(ctx, `
		UPDATE users SET limit_active = ?, limit_weekly = ?, limit_storage_bytes = ?, limit_clip_seconds = ?
		WHERE plex_user_id = ? AND role <> 'owner'
	`, nullableInt(override.Active), nullableInt(override.Weekly), nullableInt64(override.Storage),
		nullableInt64(clipSeconds), plexUserID)
	if err != nil {
		return fmt.Errorf("save user limits: %w", err)
	}

	return nil
}

// StorageUsed returns how many bytes a user's clips take.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - owner: The user's plex.tv id.
//
// Returns:
//   - used: The bytes the user's clip files take.
//   - err: Non-nil when the read fails.
func (db *DB) StorageUsed(ctx context.Context, owner int) (int64, error) {
	var used sql.NullInt64

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT SUM(output_bytes) FROM clips WHERE owner_id = ?`),
		owner,
	).Scan(&used)
	if err != nil {
		return 0, fmt.Errorf("storage used: %w", err)
	}

	return used.Int64, nil
}

// UserLimits returns one user's own render limits.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - plexUserID: The user.
//
// Returns:
//   - override: The user's limits, all nil for a user not stored.
//   - err: Non-nil when the read fails.
func (db *DB) UserLimits(ctx context.Context, plexUserID int) (quota.Override, error) {
	var limits overrideColumns

	err := db.conn.QueryRowContext(ctx, db.rewrite(`
		SELECT limit_active, limit_weekly, limit_storage_bytes, limit_clip_seconds
		FROM users WHERE plex_user_id = ?
	`), plexUserID).Scan(&limits.active, &limits.weekly, &limits.storage, &limits.clipSeconds)
	if errors.Is(err, sql.ErrNoRows) {
		return quota.Override{}, nil
	}

	if err != nil {
		return quota.Override{}, fmt.Errorf("user limits: %w", err)
	}

	return limits.override(), nil
}

// override turns the scanned columns into a user's limits.
//
// Returns:
//   - override: The limits, nil where a column is NULL.
func (limits overrideColumns) override() quota.Override {
	override := quota.Override{ClipLength: nil, Storage: nil, Active: nil, Weekly: nil}

	if limits.active.Valid {
		override.Active = new(int(limits.active.Int64))
	}

	if limits.weekly.Valid {
		override.Weekly = new(int(limits.weekly.Int64))
	}

	if limits.storage.Valid {
		override.Storage = new(limits.storage.Int64)
	}

	if limits.clipSeconds.Valid {
		override.ClipLength = new(time.Duration(limits.clipSeconds.Int64) * time.Second)
	}

	return override
}

// nullableInt writes an optional int as a column value.
//
// Parameters:
//   - value: The value, nil for NULL.
//
// Returns:
//   - column: The value, or nil for NULL.
func nullableInt(value *int) any {
	if value == nil {
		return nil
	}

	return *value
}

// nullableInt64 writes an optional int64 as a column value.
//
// Parameters:
//   - value: The value, nil for NULL.
//
// Returns:
//   - column: The value, or nil for NULL.
func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}

	return *value
}
