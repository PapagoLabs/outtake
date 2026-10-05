// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package label

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderLabel(t *testing.T, props Props) string {
	t.Helper()

	var buf strings.Builder

	err := Label(props).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

// labelClasses reads the class list off the rendered label.
//
// Parameters:
//   - body: Rendered label markup.
//
// Returns:
//   - classes: The classes the label carries, unescaped and split on spaces.
func labelClasses(body string) []string {
	raw := regexp.MustCompile(`<label[^>]*class="([^"]*)"`).FindStringSubmatch(body)

	if raw == nil {
		return nil
	}

	return strings.Fields(html.UnescapeString(raw[1]))
}

func TestLabelAssociatesWithItsControl(t *testing.T) {
	t.Parallel()

	body := renderLabel(t, Props{For: "clip-name"})

	assert.Contains(t, body, `for="clip-name"`)
	assert.Contains(t, body, `data-tui-label-disabled-style="opacity-50 cursor-not-allowed"`)
	assert.NotContains(t, body, `id=`, "an absent id leaves the attribute off")
	assert.NotContains(t, body, "text-destructive")
}

func TestLabelOmitsAnAbsentAssociation(t *testing.T) {
	t.Parallel()

	assert.NotContains(t, renderLabel(t, Props{}), "for=")
}

func TestLabelTurnsDestructiveWithAnError(t *testing.T) {
	t.Parallel()

	classes := labelClasses(renderLabel(t, Props{Error: "That CRF is out of range."}))

	assert.Contains(t, classes, "text-destructive")
	assert.Contains(t, classes, "text-sm")
	assert.Contains(t, classes, "font-medium")
	assert.Contains(t, classes, "leading-none")
	assert.Contains(t, classes, "inline-block")
}

func TestLabelRendersIdentityClassAndAttributes(t *testing.T) {
	t.Parallel()

	body := renderLabel(t, Props{
		ID:         "name-label",
		For:        "clip-name",
		Class:      "sr-only",
		Attributes: templ.Attributes{"aria-hidden": "true"},
	})

	assert.Contains(t, body, `id="name-label"`)
	assert.Contains(t, body, `aria-hidden="true"`)
	assert.Contains(t, labelClasses(body), "sr-only")
}

func TestLabelLoadsItsScript(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	require.NoError(t, Script().Render(t.Context(), &buf))
	assert.Contains(t, buf.String(), "/assets/js/label.min.js")
}
