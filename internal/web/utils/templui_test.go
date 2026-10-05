// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package utils

import (
	"errors"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingAfterWriter accepts the first budgeted writes and rejects the rest.
type failingAfterWriter struct {
	// allowed is how many writes still succeed.
	allowed int
}

// errFailedWrite is what failingAfterWriter reports once its budget is spent.
var errFailedWrite = errors.New("write failed")

// Write accepts the bytes while the budget covers this write.
//
// Parameters:
//   - data: Bytes the component tried to write.
//
// Returns:
//   - written: The length of data while the budget covers this write, otherwise
//     zero.
//   - err: errFailedWrite once the budget is spent.
func (writer *failingAfterWriter) Write(data []byte) (int, error) {
	if writer.allowed > 0 {
		writer.allowed--

		return len(data), nil
	}

	return 0, errFailedWrite
}

// The merge builds its result from a map, so the order of the classes it keeps
// is not stable between runs. Every assertion here is about which classes
// survive, never about the order they come back in.
func TestTwMergeResolvesConflicts(t *testing.T) {
	t.Parallel()

	assert.ElementsMatch(
		t,
		[]string{"p-2", "text-sm"},
		strings.Fields(TwMerge("p-4", "p-2", "text-sm")),
		"a later class wins the conflict against an earlier one",
	)
	assert.ElementsMatch(
		t,
		[]string{"p-4", "px-2"},
		strings.Fields(TwMerge("p-4", "px-2")),
		"axis-specific padding is its own class group and does not displace the shorthand",
	)
	assert.ElementsMatch(
		t,
		[]string{"hover:bg-red-500", "bg-blue-500"},
		strings.Fields(TwMerge("bg-blue-500", "hover:bg-red-500")),
		"a variant is not the same class group as its unprefixed base",
	)
	assert.ElementsMatch(t, []string{"text-sm"}, strings.Fields(TwMerge("", "text-sm")),
		"an empty class list contributes nothing")
	assert.Empty(t, TwMerge())
}

func TestIfReturnsTheZeroValueOfANonStringType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "on", If(true, "on"))
	assert.Empty(t, If(false, "on"))
	assert.True(t, If(true, true))
	assert.False(t, If(false, true))
	assert.Equal(t, 7, If(true, 7))
	assert.Zero(t, If(false, 7))
	assert.Nil(t, If(false, []string{"on"}),
		"the zero value of a slice type is nil rather than an empty literal")
	assert.Equal(t, []string{"on"}, If(true, []string{"on"}))
}

func TestIfElsePicksOneOfTwo(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "yes", IfElse(true, "yes", "no"))
	assert.Equal(t, "no", IfElse(false, "yes", "no"))
	assert.Equal(t, 1, IfElse(true, 1, 2))
	assert.Equal(t, 0, IfElse(false, 1, 0))
}

func TestRandomIDIsPrefixedAndUnpredictable(t *testing.T) {
	t.Parallel()

	first := RandomID()
	assert.Regexp(t, `^id-[A-Z2-7]{26}$`, first)
	assert.NotEqual(t, first, RandomID())
}

func TestScriptURLCarriesTheCacheBustingVersion(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/assets/js/clip.js?v="+ScriptVersion, ScriptURL("/assets/js/clip.js"))
}

func TestComponentScriptRendersTheMinifiedFile(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ComponentScript("clip").Render(t.Context(), &buf)
	require.NoError(t, err)

	assert.Contains(
		t,
		buf.String(),
		`<script type="module" src="/assets/js/clip.min.js?v=`+ScriptVersion+`">`,
	)
	assert.NotContains(t, buf.String(), "nonce",
		"a request with no nonce carries no nonce attribute")
}

// The flag this test flips is package state that every other ComponentScript
// test reads, so the test cannot run beside them.
//
//nolint:paralleltest // the test flips a package-level flag the parallel tests read.
func TestComponentScriptRendersTheUnminifiedFile(t *testing.T) {
	setUnminifiedScripts(t)

	var buf strings.Builder

	err := ComponentScript("clip").Render(t.Context(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), `src="/assets/js/clip.js?v=`+ScriptVersion+`"`)
	assert.NotContains(t, buf.String(), "clip.min.js")
}

func TestComponentScriptCarriesTheContentSecurityNonce(t *testing.T) {
	t.Parallel()

	ctx := templ.WithNonce(t.Context(), `abc"123`)
	var buf strings.Builder

	err := ComponentScript("clip").Render(ctx, &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), `<script type="module" nonce="abc&#34;123"`,
		"the nonce is escaped rather than closing the attribute early")
}

func TestComponentScriptReportsEveryWriteFailure(t *testing.T) {
	t.Parallel()

	ctx := templ.WithNonce(t.Context(), "nonce")

	for budget := range 7 {
		writer := &failingAfterWriter{allowed: budget}

		err := ComponentScript("clip").Render(ctx, writer)

		require.ErrorIsf(t, err, errFailedWrite,
			"a write rejected at step %d stops the render instead of being swallowed", budget)
	}
}

// setUnminifiedScripts switches component script loading to the unminified
// files for the duration of one test.
//
// Parameters:
//   - t: Test whose cleanup restores the flag.
func setUnminifiedScripts(t *testing.T) {
	t.Helper()

	previous := UseUnminifiedScripts

	UseUnminifiedScripts = true

	t.Cleanup(func() { UseUnminifiedScripts = previous })
}
