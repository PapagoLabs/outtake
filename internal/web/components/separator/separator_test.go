// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package separator

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderSeparator(t *testing.T, props ...Props) string {
	t.Helper()

	var buf strings.Builder

	err := Separator(props...).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

// separatorClasses reads the class list off the separator's outer element.
//
// Parameters:
//   - body: Rendered separator markup.
//
// Returns:
//   - classes: The classes the separator carries, unescaped and split on spaces.
func separatorClasses(body string) []string {
	raw := regexp.MustCompile(`<div[^>]*role="separator"[^>]*class="([^"]*)"`).
		FindStringSubmatch(body)

	if raw == nil {
		return nil
	}

	return strings.Fields(html.UnescapeString(raw[1]))
}

func TestSeparatorDefaultsToHorizontal(t *testing.T) {
	t.Parallel()

	body := renderSeparator(t)

	assert.Contains(t, body, `aria-orientation="horizontal"`)
	assert.Contains(t, body, `border-t`)
	assert.Contains(t, body, "w-full")
	assert.NotContains(t, body, `aria-orientation="vertical"`)
	assert.NotContains(t, body, `id=`, "an absent id leaves the attribute off")
}

func TestSeparatorRendersAVerticalRule(t *testing.T) {
	t.Parallel()

	body := renderSeparator(t, Props{Orientation: OrientationVertical})

	assert.Contains(t, body, `aria-orientation="vertical"`)
	assert.Contains(t, body, `border-l`)
	assert.Contains(t, body, "h-full")
	assert.Contains(t, body, "flex-col")
	assert.NotContains(t, body, `aria-orientation="horizontal"`)
}

func TestSeparatorRendersEachDecoration(t *testing.T) {
	t.Parallel()

	tests := map[Decoration]string{
		DecorationDashed: "border-dashed",
		DecorationDotted: "border-dotted",
		Decoration(""):   "",
	}

	for decoration, want := range tests {
		t.Run(string(decoration), func(t *testing.T) {
			t.Parallel()

			horizontal := renderSeparator(t, Props{Decoration: decoration})
			vertical := renderSeparator(t, Props{
				Orientation: OrientationVertical,
				Decoration:  decoration,
			})

			if want == "" {
				assert.NotContains(t, horizontal, "border-dashed")
				assert.NotContains(t, horizontal, "border-dotted")

				return
			}

			assert.Contains(t, horizontal, want)
			assert.Contains(t, vertical, want)
		})
	}
}

func TestSeparatorRendersIdentityClassAndAttributes(t *testing.T) {
	t.Parallel()

	body := renderSeparator(t, Props{
		ID:         "divider",
		Class:      "my-4",
		Attributes: templ.Attributes{"data-testid": "divider"},
	})

	assert.Contains(t, body, `id="divider"`)
	assert.Contains(t, body, `data-testid="divider"`)
	assert.Contains(t, separatorClasses(body), "my-4")
}

func TestSeparatorKeepsTheSharedShape(t *testing.T) {
	t.Parallel()

	assert.Subset(t, separatorClasses(renderSeparator(t, Props{Class: "gap-2"})),
		[]string{"shrink-0", "w-full"})
	assert.Subset(t, separatorClasses(renderSeparator(t, Props{
		Orientation: OrientationVertical,
		Class:       "gap-2",
	})), []string{"shrink-0", "h-full"})
}
