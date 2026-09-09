// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
)

// Scannable is implemented by [sql.Row] and [sql.Rows].
type Scannable interface {
	Scan(dest ...any) error
}

const (
	// ClipSelectCols is the clip table projection used by read queries.
	clipSelectCols = `id, media_id, media_title, media_type, clip_type, status, progress,
		input_path, output_path, start_time, duration, quality, width, fps,
		error_message, created_at, updated_at, name, audio_index, crop_black_bars,
		web_safe_color`
)

// ErrClipNotFound is returned when a clip row does not exist.
var ErrClipNotFound = errors.New("clip not found")

// SaveClip inserts or replaces a clip job.
func (db *DB) SaveClip(ctx context.Context, job *queue.Job) error {
	query := db.rewrite(`
		INSERT INTO clips (
			id, media_id, media_title, media_type, clip_type, status, progress,
			input_path, output_path, start_time, duration, quality, width, fps,
			error_message, created_at, updated_at, name, audio_index, crop_black_bars,
			web_safe_color
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			clip_type = excluded.clip_type,
			status = excluded.status,
			progress = excluded.progress,
			output_path = excluded.output_path,
			start_time = excluded.start_time,
			duration = excluded.duration,
			quality = excluded.quality,
			audio_index = excluded.audio_index,
			crop_black_bars = excluded.crop_black_bars,
			web_safe_color = excluded.web_safe_color,
			error_message = excluded.error_message,
			updated_at = excluded.updated_at
	`)

	//nolint:gosec // G701: rewrite maps ? to $n on constant SQL.
	_, err := db.conn.ExecContext(ctx, query,
		job.ID,
		job.MediaID,
		job.MediaTitle,
		job.MediaType,
		string(job.Type),
		string(job.Status),
		job.Progress,
		job.InputPath,
		nullString(job.OutputPath),
		job.StartTime,
		job.Duration,
		job.Quality,
		job.Width,
		job.FPS,
		nullString(job.Error),
		job.CreatedAt,
		job.UpdatedAt,
		job.Name,
		job.AudioIndex,
		cropBlackBarsColumn(job),
		webSafeColorColumn(job),
	)
	if err != nil {
		return fmt.Errorf("save clip: %w", err)
	}

	return nil
}

// GetClip loads a clip by ID.
func (db *DB) GetClip(ctx context.Context, id string) (*queue.Job, error) {
	query := db.rewrite(`SELECT ` + clipSelectCols + ` FROM clips WHERE id = ?`)

	//nolint:gosec // G701: rewrite maps ? to $n on constant SQL.
	row := db.conn.QueryRowContext(ctx, query, id)

	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrClipNotFound
		}

		return nil, fmt.Errorf("get clip: %w", err)
	}

	return job, nil
}

// ListClips returns all clips ordered by creation time, newest first.
func (db *DB) ListClips(ctx context.Context) ([]*queue.Job, error) {
	rows, err := db.conn.QueryContext(
		ctx,
		db.rewrite(`SELECT `+clipSelectCols+` FROM clips ORDER BY created_at DESC, id DESC`),
	)
	if err != nil {
		return nil, fmt.Errorf("list clips: %w", err)
	}
	defer rows.Close()

	jobs, scanErr := scanJobs(rows)
	if scanErr != nil {
		return nil, fmt.Errorf("list clips: %w", scanErr)
	}

	return jobs, nil
}

// ListPendingClips returns clips that still need processing.
func (db *DB) ListPendingClips(ctx context.Context) ([]*queue.Job, error) {
	rows, err := db.conn.QueryContext(
		ctx,
		db.rewrite(
			`SELECT `+clipSelectCols+` FROM clips WHERE status IN (?, ?) ORDER BY created_at`,
		),
		string(queue.JobStatusPending),
		string(queue.JobStatusProcessing),
	)
	if err != nil {
		return nil, fmt.Errorf("list pending clips: %w", err)
	}
	defer rows.Close()

	jobs, scanErr := scanJobs(rows)
	if scanErr != nil {
		return nil, fmt.Errorf("list pending clips: %w", scanErr)
	}

	return jobs, nil
}

// ListClipsForMedia returns clips created from a media item.
func (db *DB) ListClipsForMedia(ctx context.Context, mediaID string) ([]*queue.Job, error) {
	rows, err := db.conn.QueryContext(
		ctx,
		db.rewrite(
			`SELECT `+clipSelectCols+` FROM clips WHERE media_id = ? ORDER BY created_at DESC, id DESC`,
		),
		mediaID,
	)
	if err != nil {
		return nil, fmt.Errorf("list media clips: %w", err)
	}
	defer rows.Close()

	jobs, scanErr := scanJobs(rows)
	if scanErr != nil {
		return nil, fmt.Errorf("list media clips: %w", scanErr)
	}

	return jobs, nil
}

// DeleteClip removes a clip row.
func (db *DB) DeleteClip(ctx context.Context, id string) error {
	query := db.rewrite(`DELETE FROM clips WHERE id = ?`)

	//nolint:gosec // G701: rewrite maps ? to $n on constant SQL.
	_, err := db.conn.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete clip: %w", err)
	}

	return nil
}

// scanJob reads one clip row into a job.
func scanJob(row Scannable) (*queue.Job, error) {
	job := &queue.Job{}
	var output sql.NullString
	var errMsg sql.NullString
	var created time.Time
	var updated time.Time
	var cropBlackBars int
	var webSafeColor int

	err := row.Scan(
		&job.ID,
		&job.MediaID,
		&job.MediaTitle,
		&job.MediaType,
		&job.Type,
		&job.Status,
		&job.Progress,
		&job.InputPath,
		&output,
		&job.StartTime,
		&job.Duration,
		&job.Quality,
		&job.Width,
		&job.FPS,
		&errMsg,
		&created,
		&updated,
		&job.Name,
		&job.AudioIndex,
		&cropBlackBars,
		&webSafeColor,
	)
	if err != nil {
		return nil, fmt.Errorf("scan clip: %w", err)
	}

	job.OutputPath = output.String
	job.Error = errMsg.String
	job.CreatedAt = created
	job.UpdatedAt = updated
	job.CropBlackBars = cropBlackBars != 0
	job.WebSafeColor = webSafeColor != 0

	return job, nil
}

// scanJobs reads every remaining clip row.
func scanJobs(rows *sql.Rows) ([]*queue.Job, error) {
	jobs := make([]*queue.Job, 0)

	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan clip: %w", err)
		}

		jobs = append(jobs, job)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate clips: %w", err)
	}

	return jobs, nil
}

// cropBlackBarsColumn stores the clip crop setting as 0 or 1.
func cropBlackBarsColumn(job *queue.Job) int {
	if job.CropBlackBars {
		return 1
	}

	return 0
}

// webSafeColorColumn stores the web-safe color setting as 0 or 1.
//
// Parameters:
//   - job: Clip job to persist.
//
// Returns:
//   - value: 1 when WebSafeColor is set, otherwise 0.
func webSafeColorColumn(job *queue.Job) int {
	if job.WebSafeColor {
		return 1
	}

	return 0
}

// nullString converts an empty string into a SQL NULL.
func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{String: "", Valid: false}
	}

	return sql.NullString{String: value, Valid: true}
}
