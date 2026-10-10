// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package failure

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/metadata"
)

// errEncoderCrashed stands in for a failure no rule names.
var errEncoderCrashed = errors.New("encoder crashed")

// refPattern is the shape of a failure's reference.
var refPattern = regexp.MustCompile(`^[0-9a-f]{6}$`)

// TestDescribeGivesARefusalNoDetails covers a failure whose message explains
// it in full: the card shows the message alone, with no reference to look up.
func TestDescribeGivesARefusalNoDetails(t *testing.T) {
	t.Parallel()

	failure := Describe("clip abc (clip)", fmt.Errorf("check: %w", clip.ErrEmptyRange))

	assert.Equal(
		t,
		clip.Failure{
			Message: "The end must be after the start",
			Details: "",
			Ref:     "",
			Chain:   "check: " + clip.ErrEmptyRange.Error(),
		},
		failure,
	)
}

// TestDescribeReportsAFailureWithDetails covers a failure the message does
// not explain: the card gets the plain message, and details that name the
// version, the reference, what failed, and the error chain with Plex tokens
// removed.
func TestDescribeReportsAFailureWithDetails(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("extract: %w: GET /library?X-Plex-Token=secret-token-1", ffmpeg.ErrTimeout)

	failure := Describe("clip abc (clip)", err)

	assert.Equal(t,
		"The render ran past its time limit. Raise OUTTAKE_FFMPEG_TIMEOUT_SEC to allow longer.",
		failure.Message)
	assert.Regexp(t, refPattern, failure.Ref)

	lines := strings.Split(failure.Details, "\n")
	require.Len(t, lines, 3)
	assert.True(t, strings.HasPrefix(lines[0], "Outtake "+metadata.String()+" · "))
	assert.True(t, strings.HasSuffix(lines[0], " · ref "+failure.Ref))
	assert.Equal(t, "clip abc (clip)", lines[1])
	assert.Contains(t, lines[2], "extract: ffmpeg ran past its time limit")
	assert.NotContains(t, failure.Details, "secret-token-1", "the token never reaches the card")
	assert.Contains(t, lines[2], "X-Plex-Token="+Redacted)
	assert.Equal(t, lines[2], failure.Chain, "the log gets the same scrubbed chain")
	assert.NotContains(t, failure.Chain, "secret-token-1")
}

// TestDescribeRedactsAQuotedToken covers a token in a JSON body an error
// quotes: neither the details nor the logged chain carry it.
func TestDescribeRedactsAQuotedToken(t *testing.T) {
	t.Parallel()

	failure := Describe(
		"preview p1",
		fmt.Errorf(`decode: %w: {"X-Plex-Token":"secret-token-2"}`, errEncoderCrashed),
	)

	assert.NotContains(t, failure.Details, "secret-token-2")
	assert.NotContains(t, failure.Chain, "secret-token-2")
	assert.Contains(t, failure.Chain, `{"X-Plex-Token":"`+Redacted+`"}`)
}

// TestDescribeNamesAnUnexpectedFailure covers a failure no rule names: the
// card says something went wrong and the details carry the chain.
func TestDescribeNamesAnUnexpectedFailure(t *testing.T) {
	t.Parallel()

	failure := Describe("preview p1", errEncoderCrashed)

	assert.Equal(t, MessageUnexpected, failure.Message)
	assert.Contains(t, failure.Details, "preview p1\nencoder crashed")
}

// TestScrubRemovesEveryPlexToken covers a token in a query and in a header
// line, and text with none.
func TestScrubRemovesEveryPlexToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "query",
			text: "GET /a?X-Plex-Token=abc&b=1",
			want: "GET /a?X-Plex-Token=" + Redacted + "&b=1",
		},
		{name: "header", text: "x-plex-token: abc def", want: "x-plex-token: " + Redacted + " def"},
		{
			name: "double-quoted value",
			text: `x-plex-token: "abc"`,
			want: `x-plex-token: "` + Redacted + `"`,
		},
		{
			name: "single-quoted value",
			text: "X-Plex-Token='abc'",
			want: "X-Plex-Token='" + Redacted + "'",
		},
		{
			name: "JSON pair",
			text: `{"X-Plex-Token":"abc","b":1}`,
			want: `{"X-Plex-Token":"` + Redacted + `","b":1}`,
		},
		{
			name: "JSON pair with a space",
			text: `{"x-plex-token": "abc"}`,
			want: `{"x-plex-token": "` + Redacted + `"}`,
		},
		{name: "none", text: "nothing secret", want: "nothing secret"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, Scrub(test.text))
		})
	}
}

// TestNewRefIsShortAndVaries covers the reference's shape, and that two
// failures get different ones.
func TestNewRefIsShortAndVaries(t *testing.T) {
	t.Parallel()

	first, second := NewRef(), NewRef()

	assert.Regexp(t, refPattern, first)
	assert.Regexp(t, refPattern, second)
	assert.NotEqual(t, first, second)
}
