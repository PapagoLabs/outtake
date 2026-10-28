// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"fmt"
	"strings"
)

// Status line wording.
const (
	// msgNothingChanged is shown when a bulk action had no clip to change.
	msgNothingChanged = "No clips were changed"
	// sentenceEnd closes a sentence.
	sentenceEnd = "."
)

// bulkVerbs names what each bulk action did to a clip.
var bulkVerbs = map[bulkAction]string{
	bulkDelete:     "Deleted",
	bulkRegenerate: "Regenerated",
	bulkCancel:     "Canceled",
	bulkShare:      "Shared",
	bulkUnshare:    "Stopped sharing",
}

// bulkMessage says what a bulk action did, as the list's status line shows
// it.
//
// Each count that is not zero gets a sentence, such as "Regenerated 8 clips.
// 2 clips weren't started." followed by the limit that stopped them. One
// sentence ends without a period, as the page's other status lines do.
//
// Parameters:
//   - action: The action.
//   - result: What it did.
//   - limitMessage: The reached limit's message, empty for none.
//
// Returns:
//   - message: The status line.
func bulkMessage(action bulkAction, result bulkResult, limitMessage string) string {
	sentences := make([]string, 0)

	if result.done > 0 {
		sentences = append(
			sentences,
			fmt.Sprintf("%s %s", bulkVerbs[action], clipCount(result.done)),
		)
	}

	if result.skipped > 0 {
		sentences = append(sentences,
			fmt.Sprintf("%s %s skipped", clipCount(result.skipped), were(result.skipped)),
		)
	}

	if result.failed > 0 {
		sentences = append(sentences,
			clipCount(result.failed)+" couldn't be changed",
			"Check the Outtake log",
		)
	}

	if result.notStarted > 0 {
		sentences = append(
			sentences,
			fmt.Sprintf("%s %s started", clipCount(result.notStarted), weren(result.notStarted)),
		)
	}

	if limitMessage != "" {
		sentences = append(sentences, limitMessage)
	}

	return joinSentences(sentences)
}

// clipCount writes a number of clips.
//
// Parameters:
//   - count: How many clips.
//
// Returns:
//   - text: Such as "1 clip" or "3 clips".
func clipCount(count int) string {
	if count == 1 {
		return "1 clip"
	}

	return fmt.Sprintf("%d clips", count)
}

// joinSentences writes sentences as one status line.
//
// Parameters:
//   - sentences: The sentences, with or without a closing period.
//
// Returns:
//   - line: One sentence without a period, several each closed with one,
//     or msgNothingChanged for none.
func joinSentences(sentences []string) string {
	switch len(sentences) {
	case 0:
		return msgNothingChanged
	case 1:
		return strings.TrimSuffix(sentences[0], sentenceEnd)
	default:
		trimmed := make([]string, 0, len(sentences))

		for _, sentence := range sentences {
			trimmed = append(trimmed, strings.TrimSuffix(sentence, sentenceEnd))
		}

		return strings.Join(trimmed, sentenceEnd+" ") + sentenceEnd
	}
}

// were agrees "was" with a number of clips.
//
// Parameters:
//   - count: How many clips.
//
// Returns:
//   - verb: "was" for one clip, "were" for any other number.
func were(count int) string {
	if count == 1 {
		return "was"
	}

	return "were"
}

// weren agrees "wasn't" with a number of clips.
//
// Parameters:
//   - count: How many clips.
//
// Returns:
//   - verb: "wasn't" for one clip, "weren't" for any other number.
func weren(count int) string {
	if count == 1 {
		return "wasn't"
	}

	return "weren't"
}
