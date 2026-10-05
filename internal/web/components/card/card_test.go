// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package card

import (
	"context"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// elementTag matches the opening tag of the element a card part rendered.
var elementTag = regexp.MustCompile(`^<[a-z0-9]+[^>]*>`)

func renderPart(t *testing.T, tag string, component templ.Component) string {
	t.Helper()

	var buf strings.Builder

	ctx := templ.WithChildren(t.Context(), templ.Raw("inner"))

	err := component.Render(ctx, &buf)
	require.NoError(t, err)

	body := buf.String()
	require.True(t, strings.HasPrefix(body, "<"+tag), "expected a <%s> to lead", tag)

	return body
}

func partClasses(t *testing.T, tag, body string) []string {
	t.Helper()

	raw := elementTag.FindString(body)
	require.NotEmpty(t, raw)

	match := regexp.MustCompile(`class="([^"]*)"`).
		FindStringSubmatch(html.UnescapeString(raw))
	require.NotNil(t, match, "the <%s> carries a class list", tag)

	return strings.Fields(match[1])
}

func TestCardRendersTheShell(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Card(Props{
		ID:         "clip-card",
		Class:      "gap-4",
		Attributes: templ.Attributes{"data-testid": "card"},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `id="clip-card"`)
	assert.Contains(t, body, `data-testid="card"`)

	classes := partClasses(t, "div", body)
	assert.Subset(t, classes, []string{
		"w-full",
		"rounded-lg",
		"border",
		"bg-card",
		"text-card-foreground",
		"gap-4",
	})
}

func TestCardOmitsAnAbsentIdentity(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	require.NoError(t, Card().Render(t.Context(), &buf))

	assert.NotContains(t, buf.String(), `id=`)
}

func TestCardHeaderRenders(t *testing.T) {
	t.Parallel()

	body := renderPart(t, "div", Header(HeaderProps{ID: "card-head", Class: "text-center"}))

	assert.Contains(t, body, `id="card-head"`)
	assert.Subset(t, partClasses(t, "div", body), []string{
		"flex",
		"flex-col",
		"space-y-1.5",
		"p-6",
		"pb-0",
		"text-center",
	})
}

func TestCardTitleRenders(t *testing.T) {
	t.Parallel()

	body := renderPart(t, "h3", Title(TitleProps{ID: "card-title", Class: "uppercase"}))

	assert.Contains(t, body, `id="card-title"`)
	assert.Subset(t, partClasses(t, "h3", body), []string{
		"text-lg",
		"font-semibold",
		"leading-none",
		"tracking-tight",
		"uppercase",
	})
}

func TestCardDescriptionRenders(t *testing.T) {
	t.Parallel()

	body := renderPart(t, "p", Description(DescriptionProps{Class: "italic"}))

	assert.Subset(t, partClasses(t, "p", body), []string{
		"text-sm",
		"text-muted-foreground",
		"italic",
	})
}

func TestCardContentRenders(t *testing.T) {
	t.Parallel()

	body := renderPart(t, "div", Content(ContentProps{ID: "card-body"}))

	assert.Contains(t, body, `id="card-body"`)
	assert.Subset(t, partClasses(t, "div", body), []string{"p-6"})
}

func TestCardFooterRenders(t *testing.T) {
	t.Parallel()

	body := renderPart(t, "div", Footer(FooterProps{Class: "justify-end"}))

	assert.Subset(t, partClasses(t, "div", body), []string{
		"flex",
		"items-center",
		"p-6",
		"pt-0",
		"justify-end",
	})
}

func TestCardPartsRenderWithoutProps(t *testing.T) {
	t.Parallel()

	tests := map[string]templ.Component{
		"Header":      Header(),
		"Title":       Title(),
		"Description": Description(),
		"Content":     Content(),
		"Footer":      Footer(),
	}

	for name, component := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			ctx := context.Context(t.Context())
			require.NoError(t, component.Render(ctx, &buf))

			assert.NotContains(t, buf.String(), `id=`,
				"an absent id leaves the attribute off")
		})
	}
}
