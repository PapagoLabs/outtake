// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package progress

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderProgress(t *testing.T, props Props) string {
	t.Helper()

	var buf strings.Builder

	err := Progress(props).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

// progressBar reads the opening tag of the progress bar element.
//
// Parameters:
//   - body: Rendered progress markup.
//
// Returns:
//   - tag: The opening tag, as written.
func progressBar(body string) string {
	return regexp.MustCompile(`<div[^>]*role="progressbar"[^>]*>`).FindString(body)
}

// indicatorClasses reads the class list off the filled portion of the bar.
//
// Parameters:
//   - body: Rendered progress markup.
//
// Returns:
//   - classes: The classes the indicator carries.
func indicatorClasses(body string) string {
	return regexp.MustCompile(`<div data-tui-progress-indicator class="([^"]*)"`).
		FindStringSubmatch(body)[1]
}

func TestProgressReportsItsValue(t *testing.T) {
	t.Parallel()

	body := renderProgress(t, Props{ID: "preview-progress", Value: 40})

	tag := progressBar(body)
	require.NotEmpty(t, tag, "the bar carries the progressbar role")

	assert.Contains(t, tag, `id="preview-progress"`)
	assert.Contains(t, tag, `aria-valuemin="0"`)
	assert.Contains(t, tag, `aria-valuemax="100"`)
	assert.Contains(t, tag, `aria-valuenow="40"`)
	assert.NotContains(t, body, "mb-1",
		"a bar with neither label nor shown value has no caption row")
	assert.NotContains(t, body, "40%",
		"the value is not shown until it is asked for")
}

func TestProgressShowsTheValueWhenAsked(t *testing.T) {
	t.Parallel()

	body := renderProgress(t, Props{Value: 40, ShowValue: true})

	assert.Contains(t, body, ">40%<", "the shown value matches the bar value")
	assert.Contains(t, body, "mb-1")
}

func TestProgressGeneratesAnIdentity(t *testing.T) {
	t.Parallel()

	assert.Regexp(t, `id="id-[0-9A-Za-z]+"`, renderProgress(t, Props{Value: 1}))
}

func TestProgressHonoursAnExplicitMaximum(t *testing.T) {
	t.Parallel()

	body := renderProgress(t, Props{Value: 25, Max: 50, ShowValue: true})

	assert.Contains(t, progressBar(body), `aria-valuemax="50"`)
	assert.Contains(t, body, ">50%<", "a quarter of fifty is half the bar")
}

func TestProgressClampsTheShownValue(t *testing.T) {
	t.Parallel()

	tests := map[int]string{
		-10:  ">0%<",
		0:    ">0%<",
		150:  ">100%<",
		1000: ">100%<",
		50:   ">50%<",
	}

	for value, want := range tests {
		t.Run(strconv.Itoa(value), func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, renderProgress(t, Props{Value: value, ShowValue: true}), want)
		})
	}
}

func TestProgressOmitsAnEmptyMaximum(t *testing.T) {
	t.Parallel()

	for _, maximum := range []int{0, -1} {
		assert.Contains(t, progressBar(renderProgress(t, Props{Max: maximum})),
			`aria-valuemax="100"`)
	}
}

func TestProgressRendersItsLabel(t *testing.T) {
	t.Parallel()

	body := renderProgress(t, Props{Label: "Encoding"})

	assert.Contains(t, body, ">Encoding</span>")
	assert.Contains(t, body, "mb-1")
	assert.NotContains(t, body, "%",
		"a label alone does not show the value")
}

func TestProgressRendersTheCaptionWhenAsked(t *testing.T) {
	t.Parallel()

	body := renderProgress(t, Props{Label: "Encoding", Value: 80, ShowValue: true})

	assert.Contains(t, body, ">Encoding</span>")
	assert.Contains(t, body, ">80%</span>")
}

func TestProgressRendersEachSize(t *testing.T) {
	t.Parallel()

	tests := map[Size]string{
		SizeSm:   "h-1",
		SizeLg:   "h-4",
		Size(""): "h-2.5",
	}

	for size, want := range tests {
		t.Run(string(size), func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, indicatorClasses(renderProgress(t, Props{Size: size})), want)
		})
	}
}

func TestProgressRendersEachVariant(t *testing.T) {
	t.Parallel()

	tests := map[Variant]string{
		VariantDefault: "bg-primary",
		VariantSuccess: "bg-green-500",
		VariantDanger:  "bg-destructive",
		VariantWarning: "bg-yellow-500",
		Variant(""):    "bg-primary",
	}

	for variant, want := range tests {
		t.Run(string(variant), func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, indicatorClasses(renderProgress(t, Props{Variant: variant})), want)
		})
	}
}

func TestProgressRendersIdentityClassAndAttributes(t *testing.T) {
	t.Parallel()

	body := renderProgress(t, Props{
		ID:         "bar",
		Class:      "mt-2",
		BarClass:   "w-1/2",
		Attributes: templ.Attributes{"data-testid": "bar"},
	})

	assert.Contains(t, body, `data-testid="bar"`)
	assert.Contains(t, body, "mt-2")
	assert.Contains(t, indicatorClasses(body), "w-1/2")
	assert.Contains(t, body, `data-tui-progress-indicator`)
	assert.Contains(t, body, "rounded-full")
}

func TestProgressLoadsItsScript(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	require.NoError(t, Script().Render(t.Context(), &buf))
	assert.Contains(t, buf.String(), "/assets/js/progress.min.js")
}
