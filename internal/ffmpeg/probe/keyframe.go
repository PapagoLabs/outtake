// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// keyframeFlag marks a keyframe packet in ffprobe's packet flags.
	keyframeFlag = "K"
	// csvSeparator splits ffprobe's CSV fields.
	csvSeparator = ","
	// secondsPerMillisecond converts seconds to milliseconds.
	millisecondsPerSecond = 1000
	// keyframeReach is how far past the mark the search reads, so a keyframe
	// exactly on it is found.
	keyframeReach = time.Millisecond
	// secondsDecimals is how many decimals a time ffprobe reads carries.
	secondsDecimals = 3
	// packetFields is how many fields a packet line holds.
	packetFields = 2
)

// KeyframeBefore finds the last keyframe at or before a time.
//
// Only a window before the time is read, so a source whose keyframes are far
// apart has none to offer. The time is rounded up to the millisecond, because
// clip marks carry milliseconds, and a mark rounded down would land before
// the keyframe and seek to the one before it.
//
// Parameters:
//   - ctx: Cancellation and deadline for the read.
//   - ffprobePath: Path to the ffprobe binary.
//   - path: Media file path.
//   - at: The time the keyframe may not be after.
//   - window: How far before at the keyframe may be.
//
// Returns:
//   - keyframe: The keyframe's time, rounded up to the millisecond.
//   - found: False when no keyframe lies in the window.
//   - err: Non-nil when ffprobe cannot read the file.
//
//nolint:nonamedreturns // The named results tell the time from the flag.
func KeyframeBefore(
	ctx context.Context,
	ffprobePath, path string,
	at, window time.Duration,
) (keyframe time.Duration, found bool, err error) {
	from := max(at-window, 0)
	to := at + keyframeReach

	args := []string{
		ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags",
		"-of", "csv=p=0",
		"-read_intervals", seconds(from) + "%" + seconds(to),
		filepath.Clean(path),
	}

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(probeCtx, args[0], args[1:]...)

	output, err := cmd.Output()
	if err != nil {
		return 0, false, fmt.Errorf("find keyframe: %w", err)
	}

	keyframe, found = lastKeyframe(string(output), from, at)

	return keyframe, found, nil
}

// lastKeyframe reads ffprobe's packet list for the last keyframe in a range.
//
// Parameters:
//   - packets: ffprobe CSV lines of pts_time and flags.
//   - from: The earliest time the keyframe may be.
//   - at: The latest time the keyframe may be.
//
// Returns:
//   - keyframe: The keyframe's time, rounded up to the millisecond.
//   - found: False when no keyframe lies in the range.
//
//nolint:nonamedreturns // The named results tell the time from the flag.
func lastKeyframe(packets string, from, at time.Duration) (keyframe time.Duration, found bool) {
	for line := range strings.Lines(packets) {
		start, ok := keyframeTime(line)
		if !ok || start < from || start > at || (found && start <= keyframe) {
			continue
		}

		keyframe, found = start, true
	}

	return keyframe, found
}

// keyframeTime reads one packet line, if it is a keyframe's.
//
// Parameters:
//   - line: A CSV line of pts_time and flags.
//
// Returns:
//   - start: The keyframe's time, rounded up to the millisecond.
//   - ok: False for another packet or an unreadable time.
//
//nolint:nonamedreturns // The named results tell the time from the flag.
func keyframeTime(line string) (start time.Duration, ok bool) {
	fields := strings.Split(strings.TrimSpace(line), csvSeparator)
	if len(fields) < packetFields || !strings.HasPrefix(fields[1], keyframeFlag) {
		return 0, false
	}

	value, err := strconv.ParseFloat(fields[0], bitRateBits)
	if err != nil {
		return 0, false
	}

	return roundUpToMillisecond(value), true
}

// roundUpToMillisecond converts seconds to a duration, rounded up to the
// millisecond.
//
// Parameters:
//   - value: Seconds, as ffprobe prints them.
//
// Returns:
//   - duration: The time, never earlier than value.
func roundUpToMillisecond(value float64) time.Duration {
	return time.Duration(math.Ceil(value*millisecondsPerSecond)) * time.Millisecond
}

// seconds writes a duration as ffprobe reads a time.
//
// Parameters:
//   - value: The time.
//
// Returns:
//   - text: Seconds with millisecond precision.
func seconds(value time.Duration) string {
	return strconv.FormatFloat(value.Seconds(), 'f', secondsDecimals, bitRateBits)
}
