// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package button

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openingTag matches the opening tag of the element the button rendered.
var openingTag = regexp.MustCompile(`^<[ab][^>]*>`)

func renderButton(t *testing.T, props Props, children ...string) string {
	t.Helper()

	var buf strings.Builder

	ctx := templ.WithChildren(t.Context(), templ.Raw(strings.Join(children, "")))

	err := Button(props).Render(ctx, &buf)
	require.NoError(t, err)

	return buf.String()
}

// buttonClasses reads the class list off the rendered control.
//
// Parameters:
//   - body: Rendered button markup.
//
// Returns:
//   - classes: The classes the control carries, unescaped and split on spaces.
func buttonClasses(body string) []string {
	raw := openingTag.FindStringSubmatch(body)

	if raw == nil {
		return nil
	}

	tag := html.UnescapeString(raw[0])
	match := regexp.MustCompile(`class="([^"]*)"`).FindStringSubmatch(tag)

	if match == nil {
		return nil
	}

	return strings.Fields(match[1])
}

func TestButtonRendersAnAnchorForAnEnabledHref(t *testing.T) {
	t.Parallel()

	body := renderButton(t, Props{Href: "/clips", Target: "_blank"}, "Clips")

	assert.True(t, strings.HasPrefix(body, "<a "), "an enabled href renders a link")
	assert.Contains(t, body, `href="/clips"`)
	assert.Contains(t, body, `target="_blank"`)
	assert.Contains(t, body, ">Clips</a>")
	assert.NotContains(t, body, "type=",
		"a link carries no form type")
}

func TestButtonRendersAButtonWithoutAnHref(t *testing.T) {
	t.Parallel()

	body := renderButton(t, Props{}, "Save")

	assert.True(t, strings.HasPrefix(body, "<button "))
	assert.Contains(t, body, `type="button"`, "an unset type falls back to button")
	assert.NotContains(t, body, "<a ")
}

func TestButtonRendersAButtonForADisabledHref(t *testing.T) {
	t.Parallel()

	body := renderButton(t, Props{Href: "/clips", Disabled: true}, "Clips")

	assert.True(t, strings.HasPrefix(body, "<button "),
		"a disabled link cannot be activated, so it is not a link")
	assert.NotContains(t, body, `href=`)
	assert.Contains(t, body, "disabled")
}

func TestButtonRendersEachVariant(t *testing.T) {
	t.Parallel()

	tests := map[Variant][]string{
		VariantDefault:     {"bg-primary", "text-primary-foreground"},
		VariantDestructive: {"bg-destructive", "text-white"},
		VariantOutline:     {"bg-background", "dark:bg-input/30"},
		VariantSecondary:   {"bg-secondary", "text-secondary-foreground"},
		VariantGhost:       {"hover:bg-accent", "hover:text-accent-foreground"},
		VariantLink:        {"text-primary", "underline-offset-4"},
		Variant("unknown"): {"bg-primary", "text-primary-foreground"},
		Variant(""):        {"bg-primary", "text-primary-foreground"},
	}

	for variant, want := range tests {
		t.Run(string(variant), func(t *testing.T) {
			t.Parallel()

			assert.Subset(t,
				buttonClasses(renderButton(t, Props{Variant: variant})),
				want,
			)
		})
	}
}

func TestButtonRendersEachSize(t *testing.T) {
	t.Parallel()

	tests := map[Size][]string{
		SizeDefault:     {"h-9", "px-4"},
		SizeSm:          {"h-8", "px-3"},
		SizeLg:          {"h-10", "px-6"},
		SizeIcon:        {"size-9"},
		Size("unknown"): {"h-9", "px-4"},
		Size(""):        {"h-9", "px-4"},
	}

	for size, want := range tests {
		t.Run(string(size), func(t *testing.T) {
			t.Parallel()

			assert.Subset(t, buttonClasses(renderButton(t, Props{Size: size})), want)
		})
	}
}

func TestButtonStretchesWhenAsked(t *testing.T) {
	t.Parallel()

	assert.Contains(t, buttonClasses(renderButton(t, Props{FullWidth: true})), "w-full")
	assert.NotContains(t, buttonClasses(renderButton(t, Props{})), "w-full")
}

func TestButtonPassesThroughIdentityTypeFormAndAttributes(t *testing.T) {
	t.Parallel()

	body := renderButton(t, Props{
		ID:    "save-clip",
		Type:  TypeSubmit,
		Form:  "clip-form",
		Class: "self-start",
		Attributes: templ.Attributes{
			"formaction": "/api/clips",
		},
	}, "Save")

	assert.Contains(t, body, `id="save-clip"`)
	assert.Contains(t, body, `type="submit"`)
	assert.Contains(t, body, `form="clip-form"`)
	assert.Contains(t, body, `formaction="/api/clips"`)
	assert.Contains(t, body, "self-start")
}

func TestButtonOmitsAnAbsentFormAndID(t *testing.T) {
	t.Parallel()

	body := renderButton(t, Props{}, "Save")

	assert.NotContains(t, body, `form=`)
	assert.NotContains(t, body, `id=`)
}

func TestButtonRendersTheResetType(t *testing.T) {
	t.Parallel()

	assert.Contains(t, renderButton(t, Props{Type: TypeReset}), `type="reset"`)
}
