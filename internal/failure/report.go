// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package failure

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/metadata"
)

const (
	// refBytes is how many random bytes a failure's reference holds.
	refBytes = 3
	// fallbackRef stands in when no random reference can be read.
	fallbackRef = "000000"
	// detailsTimeLayout is how the details block writes the time.
	detailsTimeLayout = "2006-01-02 15:04:05 UTC"
	// Redacted replaces a secret in the details and the log.
	Redacted = "[redacted]"
)

// plexTokenPattern finds a Plex token in a URL query, a header line, or a
// JSON or quoted pair, with or without quotes around the name and the value.
var plexTokenPattern = regexp.MustCompile(`(?i)(x-plex-token["']?\s*[=:]\s*["']?)[^&\s"']+`)

// Describe is the clip.DescribeFunc failed renders use: the plain message
// Classify picks, and for a failure the message does not explain, details
// under a new reference.
//
// Parameters:
//   - subject: What failed, such as "clip 1234 (clip)".
//   - err: What the render returned.
//
// Returns:
//   - failure: The message, with details and a reference unless it refuses
//     input.
func Describe(subject string, err error) clip.Failure {
	chain := Scrub(err.Error())

	message, input := Classify(err)
	if input {
		return clip.Failure{Message: message, Details: "", Ref: "", Chain: chain}
	}

	ref := NewRef()

	return clip.Failure{
		Message: message,
		Details: Details(ref, subject, chain),
		Ref:     ref,
		Chain:   chain,
	}
}

// Details lays out the technical report behind a failure: the version, the
// time, and the reference on the first line, then what failed, then the
// error chain.
//
// Parameters:
//   - ref: The reference the log line carries too.
//   - subject: What failed, such as a request's method and path.
//   - chain: The error chain, already scrubbed.
//
// Returns:
//   - details: The report, one item per line.
func Details(ref, subject, chain string) string {
	return "Outtake " + metadata.String() + " · " +
		time.Now().UTC().Format(detailsTimeLayout) + " · ref " + ref + "\n" +
		subject + "\n" +
		chain
}

// NewRef returns a short random reference that ties a failure to its log
// line.
//
// Returns:
//   - ref: Six hex characters.
func NewRef() string {
	var buf [refBytes]byte

	_, err := rand.Read(buf[:])
	if err != nil {
		return fallbackRef
	}

	return hex.EncodeToString(buf[:])
}

// Scrub removes every X-Plex-Token value from text bound for a page or the
// log.
//
// Parameters:
//   - text: The text to clean.
//
// Returns:
//   - clean: The text with every token value replaced.
func Scrub(text string) string {
	return plexTokenPattern.ReplaceAllString(text, "${1}"+Redacted)
}
