// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package input

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inputTag matches the input the component rendered.
var inputTag = regexp.MustCompile(`<input[^>]*>`)

// generatedID matches an id the component generated for itself.
var generatedID = regexp.MustCompile(`id="id-[0-9A-Za-z]+"`)

func renderInput(t *testing.T, props Props) string {
	t.Helper()

	var buf strings.Builder

	err := Input(props).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

// inputAttrs reads the rendered input tag with its class list removed.
//
// Parameters:
//   - body: Rendered input markup.
//
// Returns:
//   - tag: The opening tag of the input, unescaped.
func inputAttrs(t *testing.T, body string) string {
	t.Helper()

	tag := inputTag.FindString(body)
	require.NotEmpty(t, tag, "the input rendered no input")

	return regexp.MustCompile(` class="[^"]*"`).
		ReplaceAllString(html.UnescapeString(tag), "")
}

func TestInputDefaultsToATextField(t *testing.T) {
	t.Parallel()

	tag := inputAttrs(t, renderInput(t, Props{}))

	assert.Contains(t, tag, `type="text"`)
	assert.NotContains(t, tag, `name=`)
	assert.NotContains(t, tag, "placeholder=")
	assert.NotContains(t, tag, "value=")
	assert.NotContains(t, tag, "disabled")
	assert.NotContains(t, tag, "readonly")
	assert.NotContains(t, tag, "required")
	assert.NotContains(t, tag, "aria-invalid")
	assert.NotContains(t, tag, "accept=")
	assert.NotContains(t, tag, "form=")
	assert.True(t, generatedID.MatchString(tag), "an absent id is generated")
}

func TestInputRendersItsFields(t *testing.T) {
	t.Parallel()

	tag := inputAttrs(t, renderInput(t, Props{
		ID:          "clip-name",
		Type:        TypeNumber,
		Name:        "crf",
		Placeholder: "23",
		Value:       "20",
		Form:        "clip-form",
		Required:    true,
	}))

	assert.Contains(t, tag, `id="clip-name"`)
	assert.Contains(t, tag, `type="number"`)
	assert.Contains(t, tag, `name="crf"`)
	assert.Contains(t, tag, `placeholder="23"`)
	assert.Contains(t, tag, `value="20"`)
	assert.Contains(t, tag, `form="clip-form"`)
	assert.Contains(t, tag, "required")
}

func TestInputRendersDisabledAndReadonly(t *testing.T) {
	t.Parallel()

	tag := inputAttrs(t, renderInput(t, Props{Disabled: true, Readonly: true}))

	assert.Contains(t, tag, "disabled")
	assert.Contains(t, tag, "readonly")
}

func TestInputMarksAnInvalidField(t *testing.T) {
	t.Parallel()

	body := renderInput(t, Props{HasError: true})

	assert.Contains(t, inputAttrs(t, body), `aria-invalid="true"`)
	assert.Contains(t, body, "border-destructive")
	assert.Contains(t, body, "ring-destructive/20")
}

func TestInputAcceptsFilesOnlyOnAFileField(t *testing.T) {
	t.Parallel()

	file := inputAttrs(t, renderInput(t, Props{
		Type:       TypeFile,
		FileAccept: "video/mp4",
	}))
	assert.Contains(t, file, `accept="video/mp4"`)

	text := inputAttrs(t, renderInput(t, Props{
		Type:       TypeText,
		FileAccept: "video/mp4",
	}))
	assert.NotContains(t, text, "accept=",
		"a text field cannot accept files")
}

func TestInputPassesAttributesThrough(t *testing.T) {
	t.Parallel()

	tag := inputAttrs(t, renderInput(t, Props{
		Attributes: templ.Attributes{"min": "0", "max": "51"},
	}))

	assert.Contains(t, tag, `min="0"`)
	assert.Contains(t, tag, `max="51"`)
}

func TestInputOffersThePasswordToggle(t *testing.T) {
	t.Parallel()

	body := renderInput(t, Props{
		ID:   "plex-token",
		Type: TypePassword,
	})

	assert.Contains(t, body, `data-tui-input-toggle-password="plex-token"`)
	assert.Contains(t, body, "icon-open")
	assert.Contains(t, body, "icon-closed")
	assert.Contains(t, body, "pr-8", "the field leaves room for the toggle")
}

func TestInputOmitsThePasswordToggleWhenAsked(t *testing.T) {
	t.Parallel()

	body := renderInput(t, Props{
		ID:               "plex-token",
		Type:             TypePassword,
		NoTogglePassword: true,
	})

	assert.NotContains(t, body, "data-tui-input-toggle-password")
	assert.NotContains(t, body, "pr-8")
}

func TestInputOmitsThePasswordToggleOnOtherTypes(t *testing.T) {
	t.Parallel()

	body := renderInput(t, Props{Type: TypeEmail})

	assert.NotContains(t, body, "data-tui-input-toggle-password")
	assert.NotContains(t, body, "pr-8")
}

func TestInputLoadsItsScript(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	require.NoError(t, Script().Render(t.Context(), &buf))
	assert.Contains(t, buf.String(), "/assets/js/input.min.js")
}
