// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package checkbox

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inputTag matches the checkbox input the component rendered.
var inputTag = regexp.MustCompile(`<input[^>]*>`)

func renderCheckbox(t *testing.T, props Props) string {
	t.Helper()

	var buf strings.Builder

	err := Checkbox(props).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

// checkboxInput reads the rendered checkbox input tag without its class list.
//
// Parameters:
//   - body: Rendered checkbox markup.
//
// Returns:
//   - tag: The opening tag of the input, unescaped, with the class list removed.
func checkboxInput(t *testing.T, body string) string {
	t.Helper()

	tag := inputTag.FindString(body)
	require.NotEmpty(t, tag, "the checkbox rendered no input")

	return regexp.MustCompile(` class="[^"]*"`).
		ReplaceAllString(html.UnescapeString(tag), "")
}

func TestCheckboxDefaultsToAnUncheckedBox(t *testing.T) {
	t.Parallel()

	tag := checkboxInput(t, renderCheckbox(t, Props{}))

	assert.Contains(t, tag, `type="checkbox"`)
	assert.Contains(t, tag, `value="on"`, "an absent value posts the HTML default")
	assert.NotContains(t, tag, "checked")
	assert.NotContains(t, tag, "disabled")
	assert.NotContains(t, tag, `name=`)
	assert.NotContains(t, tag, `id=`)
	assert.NotContains(t, tag, "form=")
	assert.NotContains(t, tag, "data-tui-checkbox")
}

func TestCheckboxRendersChecked(t *testing.T) {
	t.Parallel()

	assert.Contains(t, checkboxInput(t, renderCheckbox(t, Props{Checked: true})), "checked")
}

func TestCheckboxRendersDisabled(t *testing.T) {
	t.Parallel()

	assert.Contains(t, checkboxInput(t, renderCheckbox(t, Props{Disabled: true})), "disabled")
}

func TestCheckboxRendersItsIdentityAndValue(t *testing.T) {
	t.Parallel()

	tag := checkboxInput(t, renderCheckbox(t, Props{
		ID:    "trim-bars",
		Name:  "cropBlackBars",
		Value: "1",
	}))

	assert.Contains(t, tag, `id="trim-bars"`)
	assert.Contains(t, tag, `name="cropBlackBars"`)
	assert.Contains(t, tag, `value="1"`)
}

func TestCheckboxRendersTheGroupMarkers(t *testing.T) {
	t.Parallel()

	tag := checkboxInput(t, renderCheckbox(t, Props{
		Group:       "types",
		GroupParent: true,
		Form:        "clip-form",
	}))

	assert.Contains(t, tag, `data-tui-checkbox-group="types"`)
	assert.Contains(t, tag, `data-tui-checkbox-parent="true"`)
	assert.Contains(t, tag, `form="clip-form"`)
}

func TestCheckboxRendersTheDefaultCheckMark(t *testing.T) {
	t.Parallel()

	body := renderCheckbox(t, Props{})

	assert.Contains(t, body, "peer-checked:opacity-100")
	assert.Contains(t, body, "peer-indeterminate:opacity-100")
	assert.Contains(t, body, `data-lucide="icon"`)
	assert.Contains(t, body, "size-3.5")
}

func TestCheckboxRendersASuppliedIcon(t *testing.T) {
	t.Parallel()

	body := renderCheckbox(t, Props{
		Icon: templ.Raw(`<span data-testid="custom-mark"></span>`),
	})

	assert.Contains(t, body, `data-testid="custom-mark"`)
	assert.Equal(t, 1, strings.Count(body, `data-lucide="icon"`),
		"only the indeterminate mark falls back to the built-in icon")
}

func TestCheckboxKeepsTheSharedShape(t *testing.T) {
	t.Parallel()

	tag := inputTag.FindString(renderCheckbox(t, Props{Class: "rounded border-input"}))

	for _, class := range []string{
		"peer",
		"size-4",
		"shrink-0",
		"border-input",
		"appearance-none",
		"cursor-pointer",
		"rounded",
	} {
		assert.Contains(t, tag, class)
	}
}

func TestCheckboxLoadsItsScript(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	require.NoError(t, Script().Render(t.Context(), &buf))
	assert.Contains(t, buf.String(), "/assets/js/checkbox.min.js")
}
