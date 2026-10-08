// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package progress

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/PapagoLabs/outtake/internal/timecode"
)

// key is the context key for a progress callback.
type key struct{}

// Writer parses ffmpeg stderr and reports percent complete. It keeps only the
// tail of what it was written, for error logs, and parses each write once, so a
// long encode costs the same per write as a short one.
type Writer struct {
	// duration is the clip length percent is computed against.
	duration time.Duration
	// on receives each new percent.
	on func(int)
	// tail holds the last TailSize bytes written.
	tail []byte
	// partial holds the unfinished line the last write ended with.
	partial []byte
	// written counts every byte written.
	written int
	// last is the percent most recently reported, -1 before the first.
	last int
}

const (
	// percentScale converts a duration ratio into a percentage.
	percentScale = 100

	// maxReportedPercent caps in-progress reports below completion.
	maxReportedPercent = 99

	// hmsSeparator rejoins an ffmpeg time= match into a parseable timestamp.
	hmsSeparator = ":"

	// TailSize is how much of ffmpeg's standard error a Writer keeps.
	TailSize = 64 << 10

	// maxPartial bounds the unfinished line carried between writes. A progress
	// line is far shorter, so a longer run without a line break holds no
	// time= worth waiting for.
	maxPartial = 256

	// noReport marks a writer that has reported nothing yet.
	noReport = -1
)

// timePattern matches ffmpeg time=HH:MM:SS.ss progress lines.
var timePattern = regexp.MustCompile(`time=(\d+):(\d+):(\d+(?:\.\d+)?)`)

// WithProgress attaches a progress callback to the context.
//
// Parameters:
//   - ctx: Parent context.
//   - fn: Callback that receives a 0-99 percent complete value.
//
// Returns:
//   - ctx: A child context that carries fn.
func WithProgress(ctx context.Context, fn func(percent int)) context.Context {
	return context.WithValue(ctx, key{}, fn)
}

// From returns the progress callback stored on ctx, if any.
//
// Parameters:
//   - ctx: Context that may carry a [WithProgress] callback.
//
// Returns:
//   - fn: The callback, or nil when none is stored.
func From(ctx context.Context) func(int) {
	fn, ok := ctx.Value(key{}).(func(int))
	if !ok {
		return nil
	}

	return fn
}

// NewWriter returns a stderr writer that reports encode progress.
//
// Parameters:
//   - duration: Clip duration used to compute percent.
//   - on: Optional callback invoked with 0-99 percent complete.
//
// Returns:
//   - writer: A writer that implements [io.Writer] and parses ffmpeg time=
//     lines.
func NewWriter(duration time.Duration, on func(int)) *Writer {
	return &Writer{
		duration: duration,
		on:       on,
		tail:     nil,
		partial:  nil,
		written:  0,
		last:     noReport,
	}
}

// Len returns how many bytes were written, including any no longer kept.
//
// Returns:
//   - n: The total written.
func (writer *Writer) Len() int {
	return writer.written
}

// String returns the kept tail of standard error.
//
// Returns:
//   - text: The last TailSize bytes written.
func (writer *Writer) String() string {
	return string(writer.tail)
}

// Write implements [io.Writer] and reports ffmpeg time= progress.
//
// Parameters:
//   - chunk: Chunk of ffmpeg standard error.
//
// Returns:
//   - written: Always len(chunk).
//   - err: Always nil.
func (writer *Writer) Write(chunk []byte) (int, error) {
	writer.written += len(chunk)
	writer.keepTail(chunk)
	writer.report(chunk)

	return len(chunk), nil
}

// carry keeps the unfinished end of a chunk for the next write, up to
// maxPartial bytes.
//
// Parameters:
//   - rest: Bytes after the last line break.
func (writer *Writer) carry(rest []byte) {
	if len(rest) > maxPartial {
		rest = rest[len(rest)-maxPartial:]
	}

	writer.partial = append(writer.partial[:0:0], rest...)
}

// keepTail appends a chunk to the tail, dropping the oldest bytes past
// TailSize.
//
// Parameters:
//   - chunk: Chunk of ffmpeg standard error.
func (writer *Writer) keepTail(chunk []byte) {
	if len(chunk) >= TailSize {
		writer.tail = append(writer.tail[:0], chunk[len(chunk)-TailSize:]...)

		return
	}

	writer.tail = append(writer.tail, chunk...)

	if over := len(writer.tail) - TailSize; over > 0 {
		kept := copy(writer.tail, writer.tail[over:])

		writer.tail = writer.tail[:kept]
	}
}

// percent reads the last time= in complete lines as percent of the clip.
//
// Parameters:
//   - lines: Complete lines of ffmpeg standard error.
//
// Returns:
//   - percent: Percent complete, capped below completion.
//   - ok: False when the lines carry no readable time=.
func (writer *Writer) percent(lines []byte) (int, bool) {
	matches := timePattern.FindAllSubmatch(lines, -1)
	if len(matches) == 0 {
		return 0, false
	}

	last := matches[len(matches)-1]

	parts := make([]string, 0, len(last)-1)
	for _, part := range last[1:] {
		parts = append(parts, string(part))
	}

	elapsed, err := timecode.Parse(strings.Join(parts, hmsSeparator))
	if err != nil {
		return 0, false
	}

	percent := int(elapsed.Duration().Seconds() / writer.duration.Seconds() * percentScale)

	return min(max(percent, 0), maxReportedPercent), true
}

// report parses the lines a chunk completes and publishes a changed percent.
// ffmpeg ends a progress line with a carriage return and other lines with a
// newline, so either one completes a line.
//
// Parameters:
//   - chunk: Chunk of ffmpeg standard error.
func (writer *Writer) report(chunk []byte) {
	if writer.on == nil || writer.duration <= 0 {
		return
	}

	text := make([]byte, 0, len(writer.partial)+len(chunk))

	text = append(text, writer.partial...)
	text = append(text, chunk...)

	end := bytes.LastIndexAny(text, "\r\n")
	writer.carry(text[end+1:])

	if end < 0 {
		return
	}

	percent, ok := writer.percent(text[:end+1])
	if !ok || percent == writer.last {
		return
	}

	writer.last = percent
	writer.on(percent)
}
