// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderErrorPage(t *testing.T, props ErrorPageProps) string {
	t.Helper()

	var buf strings.Builder

	err := ErrorPage(props).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

func TestErrorPageShowsTheSuppliedTitleAndMessage(t *testing.T) {
	t.Parallel()

	body := renderErrorPage(t, ErrorPageProps{
		Title:   "Not found",
		Message: "That clip is gone.",
		Status:  404,
	})

	assert.Contains(t, body, "<title>Not found - Outtake</title>")
	assert.Contains(t, body, `<h1 class="text-2xl font-bold tracking-tight">Not found</h1>`)
	assert.Contains(t, body, `<p class="text-muted-foreground">That clip is gone.</p>`)
}

func TestErrorPageRendersAnEmptyMessage(t *testing.T) {
	t.Parallel()

	body := renderErrorPage(t, ErrorPageProps{Title: "Something broke", Status: 500})

	assert.Contains(t, body, "<title>Something broke - Outtake</title>")
	assert.Contains(t, body, `<p class="text-muted-foreground"></p>`,
		"an absent message leaves the paragraph rather than dropping it")
	assert.NotContains(t, body, "Outtake</p>",
		"nothing is invented in place of the supplied message")
}

func TestErrorPageOffersTheWayBack(t *testing.T) {
	t.Parallel()

	body := renderErrorPage(t, ErrorPageProps{Title: "Gone", Message: "No such item."})

	assert.Contains(t, body, `<a href="/"`)
	assert.Contains(t, body, "Back to Outtake")
	assert.Contains(t, body, `src="/assets/brand/mascot-128.png"`)
}

func TestErrorPageEscapesTheSuppliedCopy(t *testing.T) {
	t.Parallel()

	body := renderErrorPage(t, ErrorPageProps{
		Title:   "<script>alert(1)</script>",
		Message: `Tom & "Jerry"`,
	})

	assert.NotContains(t, body, "<script>alert(1)</script>")
	assert.Contains(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assert.Contains(t, body, "Tom &amp; &#34;Jerry&#34;")
}
