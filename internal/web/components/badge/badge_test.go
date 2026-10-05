// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package badge

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderBadge(t *testing.T, props ...Props) string {
	t.Helper()

	var buf strings.Builder

	err := Badge(props...).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

// badgeClasses reads the class list off the rendered badge as whole tokens.
//
// Parameters:
//   - body: Rendered badge markup.
//
// Returns:
//   - classes: The classes the badge carries, unescaped and split on spaces.
func badgeClasses(body string) []string {
	raw := regexp.MustCompile(`<span[^>]*class="([^"]*)"`).FindStringSubmatch(body)

	if raw == nil {
		return nil
	}

	return strings.Fields(html.UnescapeString(raw[1]))
}

func TestBadgeRendersEachVariant(t *testing.T) {
	t.Parallel()

	tests := map[Variant]struct {
		classes []string
		fill    string
	}{
		VariantDefault:     {[]string{"bg-primary", "text-primary-foreground"}, "bg-primary"},
		VariantSecondary:   {[]string{"bg-secondary", "text-secondary-foreground"}, "bg-secondary"},
		VariantDestructive: {[]string{"bg-destructive", "text-white"}, "bg-destructive"},
		VariantOutline:     {[]string{"text-foreground"}, ""},
		Variant("unknown"): {[]string{"bg-primary", "text-primary-foreground"}, "bg-primary"},
		Variant(""):        {[]string{"bg-primary", "text-primary-foreground"}, "bg-primary"},
	}

	fills := []string{"bg-primary", "bg-secondary", "bg-destructive"}

	for variant, want := range tests {
		t.Run(string(variant), func(t *testing.T) {
			t.Parallel()

			classes := badgeClasses(renderBadge(t, Props{Variant: variant}))

			assert.Subset(t, classes, want.classes)

			for _, fill := range fills {
				if fill == want.fill {
					assert.Contains(t, classes, fill)

					continue
				}

				assert.NotContains(t, classes, fill,
					"a variant carries only its own fill")
			}
		})
	}

	assert.Subset(t,
		badgeClasses(renderBadge(t, Props{Variant: VariantDefault})),
		[]string{"[a&]:hover:bg-primary/90", "border-transparent"},
	)
	assert.Subset(t,
		badgeClasses(renderBadge(t, Props{Variant: VariantSecondary})),
		[]string{"[a&]:hover:bg-secondary/90"},
	)
	assert.Subset(t,
		badgeClasses(renderBadge(t, Props{Variant: VariantDestructive})),
		[]string{"[a&]:hover:bg-destructive/90"},
	)
	assert.Subset(t,
		badgeClasses(renderBadge(t, Props{Variant: VariantOutline})),
		[]string{"[a&]:hover:bg-accent", "[a&]:hover:text-accent-foreground"},
	)
}

func TestBadgeKeepsTheSharedShape(t *testing.T) {
	t.Parallel()

	classes := badgeClasses(renderBadge(t))

	assert.Subset(t, classes, []string{
		"inline-flex",
		"items-center",
		"justify-center",
		"rounded-md",
		"border",
		"px-2",
		"py-0.5",
		"text-xs",
		"font-medium",
		"w-fit",
		"whitespace-nowrap",
		"shrink-0",
	})
}

func TestBadgeRendersIdentityClassAndAttributes(t *testing.T) {
	t.Parallel()

	body := renderBadge(t, Props{
		ID:      "clip-status",
		Variant: VariantSecondary,
		Class:   "uppercase",
		Attributes: templ.Attributes{
			"aria-current": "true",
		},
	})

	assert.Contains(t, body, `id="clip-status"`)
	assert.Contains(t, body, `aria-current="true"`)
	assert.Subset(t, badgeClasses(body), []string{"uppercase", "bg-secondary"})
}

func TestBadgeOmitsAnAbsentID(t *testing.T) {
	t.Parallel()

	assert.NotContains(t, renderBadge(t, Props{Variant: VariantOutline}), `id=`)
}
