// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package progress

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PapagoLabs/outtake/internal/timecode"
)

// key is the context key for a progress callback.
type key struct{}

// Writer parses ffmpeg stderr and reports percent complete.
type Writer struct {
	duration time.Duration
	on       func(int)
	buf      bytes.Buffer
}

const (
	// percentScale converts a duration ratio into a percentage.
	percentScale = 100

	// maxReportedPercent caps in-progress reports below completion.
	maxReportedPercent = 99

	// hmsSeparator rejoins an ffmpeg time= match into a parseable timestamp.
	hmsSeparator = ":"
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
		buf:      bytes.Buffer{},
	}
}

// Len returns the number of captured stderr bytes.
//
// Returns:
//   - n: The captured buffer length.
func (writer *Writer) Len() int {
	return writer.buf.Len()
}

// String returns the captured stderr text.
//
// Returns:
//   - text: Bytes written so far as a string.
func (writer *Writer) String() string {
	return writer.buf.String()
}

// Write implements [io.Writer] and reports ffmpeg time= progress.
//
// Parameters:
//   - p: Chunk of ffmpeg standard error.
//
// Returns:
//   - written: Bytes captured into the buffer.
//   - err: Non-nil when the buffer write failed.
func (writer *Writer) Write(p []byte) (int, error) {
	written, err := writer.buf.Write(p)
	if err != nil {
		return written, fmt.Errorf("progress write: %w", err)
	}

	writer.report()

	return written, nil
}

// report publishes the latest parsed percent to the callback.
func (writer *Writer) report() {
	if writer.on == nil || writer.duration <= 0 {
		return
	}

	matches := timePattern.FindAllStringSubmatch(writer.buf.String(), -1)
	if len(matches) == 0 {
		return
	}

	last := matches[len(matches)-1]

	elapsed, err := timecode.Parse(strings.Join(last[1:], hmsSeparator))
	if err != nil {
		return
	}

	percent := int(elapsed.Duration().Seconds() / writer.duration.Seconds() * percentScale)

	percent = min(max(percent, 0), maxReportedPercent)

	writer.on(percent)
}
