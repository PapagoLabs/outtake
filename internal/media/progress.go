// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// progressKey is the context key for a progress callback.
type progressKey struct{}

// progressWriter parses ffmpeg stderr and reports percent complete.
type progressWriter struct {
	duration float64
	on       func(int)
	buf      bytes.Buffer
}

const (
	// PercentScale converts a duration ratio into a percentage.
	percentScale = 100

	// MaxReportedPercent caps in-progress reports below completion.
	maxReportedPercent = 99

	// SecondsBitSize is the bit size used when parsing ffmpeg seconds.
	secondsBitSize = 64

	// SecondsPerHour is the number of seconds in one hour.
	secondsPerHour = 3600

	// SecondsPerMinute is the number of seconds in one minute.
	secondsPerMinute = 60
)

// timePattern matches ffmpeg time=HH:MM:SS.ss progress lines.
var timePattern = regexp.MustCompile(`time=(\d+):(\d+):(\d+(?:\.\d+)?)`)

// WithProgress attaches a progress callback to the context.
func WithProgress(ctx context.Context, fn func(percent int)) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

// progressFrom returns the progress callback stored on ctx, if any.
func progressFrom(ctx context.Context) func(int) {
	fn, ok := ctx.Value(progressKey{}).(func(int))
	if !ok {
		return nil
	}

	return fn
}

// Write implements [io.Writer] and reports ffmpeg time= progress.
func (writer *progressWriter) Write(p []byte) (int, error) {
	written, err := writer.buf.Write(p)
	if err != nil {
		return written, fmt.Errorf("progress write: %w", err)
	}

	writer.report()

	return written, nil
}

// report publishes the latest parsed percent to the callback.
func (writer *progressWriter) report() {
	if writer.on == nil || writer.duration <= 0 {
		return
	}

	matches := timePattern.FindAllStringSubmatch(writer.buf.String(), -1)
	if len(matches) == 0 {
		return
	}

	last := matches[len(matches)-1]
	seconds := parseHMS(last[1], last[2], last[3])
	percent := int(seconds / writer.duration * percentScale)

	switch {
	case percent > maxReportedPercent:
		percent = maxReportedPercent
	case percent < 0:
		percent = 0
	default:
	}

	writer.on(percent)
}

// parseHMS converts an ffmpeg timestamp into seconds.
func parseHMS(hours, minutes, seconds string) float64 {
	parsedHours, err := strconv.Atoi(hours)
	if err != nil {
		parsedHours = 0
	}

	parsedMinutes, err := strconv.Atoi(minutes)
	if err != nil {
		parsedMinutes = 0
	}

	parsedSeconds, err := strconv.ParseFloat(seconds, secondsBitSize)
	if err != nil {
		parsedSeconds = 0
	}

	return float64(
		parsedHours,
	)*secondsPerHour + float64(
		parsedMinutes,
	)*secondsPerMinute + parsedSeconds
}
