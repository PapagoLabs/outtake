// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

// scannable is one row source a query can scan a record out of.
type scannable interface {
	Scan(dest ...any) error
}

const (
	// clipSelectCols is the column projection shared by every clip read query.
	clipSelectCols = `id, media_id, media_title, media_type, clip_type, status, progress,
		input_path, output_path, start_time, duration, quality, width, fps,
		error_message, created_at, updated_at, name, audio_index, crop_black_bars,
		web_safe_color, preserve_hdr`
)

// ErrClipNotFound is returned when a clip row does not exist.
var ErrClipNotFound = errors.New("clip not found")

// SaveClip inserts or replaces a job.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - job: The render to persist.
//
// Returns:
//   - err: Non-nil when the row cannot be written.
func (db *DB) SaveClip(ctx context.Context, job *clip.Job) error {
	query := db.rewrite(`
		INSERT INTO clips (
			id, media_id, media_title, media_type, clip_type, status, progress,
			input_path, output_path, start_time, duration, quality, width, fps,
			error_message, created_at, updated_at, name, audio_index, crop_black_bars,
			web_safe_color, preserve_hdr
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			clip_type = excluded.clip_type,
			status = excluded.status,
			progress = excluded.progress,
			output_path = excluded.output_path,
			start_time = excluded.start_time,
			duration = excluded.duration,
			quality = excluded.quality,
			width = excluded.width,
			fps = excluded.fps,
			audio_index = excluded.audio_index,
			crop_black_bars = excluded.crop_black_bars,
			web_safe_color = excluded.web_safe_color,
			preserve_hdr = excluded.preserve_hdr,
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
		job.StartTime.Seconds(),
		job.Duration.Seconds(),
		job.Quality,
		job.Width,
		job.FPS,
		nullString(job.Error),
		job.CreatedAt,
		job.UpdatedAt,
		job.Name,
		job.AudioIndex,
		cropBlackBarsColumn(&job.Clip),
		webSafeColorColumn(&job.Clip),
		preserveHDRColumn(&job.Clip),
	)
	if err != nil {
		return fmt.Errorf("save clip: %w", err)
	}

	return nil
}

// GetClip loads a job by ID.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - id: Identifier of the job to load.
//
// Returns:
//   - job: The stored render.
//   - err: ErrClipNotFound when no row matches, otherwise the read failure.
func (db *DB) GetClip(ctx context.Context, id string) (*clip.Job, error) {
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

// ListClips returns every job ordered by creation time, newest first.
//
// Parameters:
//   - ctx: Request scope for the read.
//
// Returns:
//   - jobs: The stored renders.
//   - err: Non-nil when the rows cannot be read.
func (db *DB) ListClips(ctx context.Context) ([]*clip.Job, error) {
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

// ListPendingClips returns jobs that still need processing.
//
// Parameters:
//   - ctx: Request scope for the read.
//
// Returns:
//   - jobs: The pending and processing renders, oldest first.
//   - err: Non-nil when the rows cannot be read.
func (db *DB) ListPendingClips(ctx context.Context) ([]*clip.Job, error) {
	rows, err := db.conn.QueryContext(
		ctx,
		db.rewrite(
			`SELECT `+clipSelectCols+` FROM clips WHERE status IN (?, ?) ORDER BY created_at`,
		),
		string(clip.StatusPending),
		string(clip.StatusProcessing),
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

// ListClipsForMedia returns the jobs created from a media item.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - mediaID: Identifier of the source media item.
//
// Returns:
//   - jobs: The renders made from that item.
//   - err: Non-nil when the rows cannot be read.
func (db *DB) ListClipsForMedia(ctx context.Context, mediaID string) ([]*clip.Job, error) {
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
//
// Parameters:
//   - ctx: Request scope for the write.
//   - id: Identifier of the clip to remove.
//
// Returns:
//   - err: Non-nil when the row cannot be deleted.
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
//
// Parameters:
//   - row: The row to scan, matching clipSelectCols.
//
// Returns:
//   - job: The decoded render.
//   - err: Non-nil when a column cannot be scanned.
func scanJob(row scannable) (*clip.Job, error) {
	job := &clip.Job{}
	var output sql.NullString
	var errMsg sql.NullString
	var startTime float64
	var duration float64
	var created time.Time
	var updated time.Time
	var cropBlackBars int
	var webSafeColor int
	var preserveHDR int

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
		&startTime,
		&duration,
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
		&preserveHDR,
	)
	if err != nil {
		return nil, fmt.Errorf("scan clip: %w", err)
	}

	job.OutputPath = output.String
	job.Error = errMsg.String
	job.StartTime = timecode.FromSeconds(startTime).Duration()
	job.Duration = timecode.FromSeconds(duration).Duration()
	job.CreatedAt = created
	job.UpdatedAt = updated
	job.CropBlackBars = cropBlackBars != 0
	job.WebSafeColor = webSafeColor != 0
	job.PreserveHDR = preserveHDR != 0

	return job, nil
}

// scanJobs reads every remaining clip row.
//
// Parameters:
//   - rows: The rows to scan, matching clipSelectCols.
//
// Returns:
//   - jobs: The decoded renders.
//   - err: Non-nil when a row cannot be scanned or the iteration fails.
func scanJobs(rows *sql.Rows) ([]*clip.Job, error) {
	jobs := make([]*clip.Job, 0)

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
//
// Parameters:
//   - record: Clip whose crop setting is stored.
//
// Returns:
//   - value: 1 when CropBlackBars is set, otherwise 0.
func cropBlackBarsColumn(record *clip.Clip) int {
	if record.CropBlackBars {
		return 1
	}

	return 0
}

// webSafeColorColumn stores the web-safe color setting as 0 or 1.
//
// Parameters:
//   - record: Clip whose color setting is stored.
//
// Returns:
//   - value: 1 when WebSafeColor is set, otherwise 0.
func webSafeColorColumn(record *clip.Clip) int {
	if record.WebSafeColor {
		return 1
	}

	return 0
}

// preserveHDRColumn stores the preserve-HDR setting as 0 or 1.
//
// Parameters:
//   - record: Clip whose HDR setting is stored.
//
// Returns:
//   - value: 1 when PreserveHDR is set, otherwise 0.
func preserveHDRColumn(record *clip.Clip) int {
	if record.PreserveHDR {
		return 1
	}

	return 0
}

// nullString converts an empty string into a SQL NULL.
//
// Parameters:
//   - value: The string to store.
//
// Returns:
//   - nullable: An invalid NullString when value is empty.
func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{String: "", Valid: false}
	}

	return sql.NullString{String: value, Valid: true}
}
