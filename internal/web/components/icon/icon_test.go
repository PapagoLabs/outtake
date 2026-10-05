// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package icon

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// iconNamePattern finds every icon name the generated registry binds.
var iconNamePattern = regexp.MustCompile(`= Icon\("([^"]+)"\)`)

func TestIconRendersTheGlyphForAName(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	require.NoError(
		t,
		Icon("check")(Props{Class: "size-4 icon-test-glyph"}).Render(t.Context(), &buf),
	)

	rendered := buf.String()
	assert.True(t, strings.HasPrefix(rendered, `<svg xmlns="http://www.w3.org/2000/svg"`))
	assert.Contains(t, rendered, `class="size-4 icon-test-glyph"`)
	assert.Contains(t, rendered, `data-lucide="icon"`)
	assert.Contains(t, rendered, `<path d="M20 6 9 17l-5-5" />`)
	assert.True(t, strings.HasSuffix(rendered, "</svg>"))
}

func TestIconUsesTheZeroPropsWhenGivenNone(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	require.NoError(t, Icon("circle")().Render(t.Context(), &buf))

	rendered := buf.String()
	assert.Contains(t, rendered, `class=""`,
		"an icon rendered without props carries an empty class attribute")
	assert.Contains(t, rendered, `<circle cx="12" cy="12" r="10" />`)
}

func TestIconIgnoresEveryPropAfterTheFirst(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Icon("circle")(Props{Class: "size-4 icon-test-first"}, Props{Class: "size-8"}).
		Render(t.Context(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), `class="size-4 icon-test-first"`)
	assert.NotContains(t, buf.String(), "size-8")
}

func TestIconReusesTheCachedGlyphOnASecondRender(t *testing.T) {
	resetIconCache(t)

	cacheKey := "square|cl:size-4 icon-test-cache"
	require.Empty(t, cachedIcon(t, cacheKey))

	var first, second strings.Builder

	require.NoError(t, Icon("square")(Props{Class: "size-4 icon-test-cache"}).
		Render(t.Context(), &first))
	require.NotEmpty(t, cachedIcon(t, cacheKey),
		"the first render of an icon-prop pair fills the cache")

	require.NoError(t, Icon("square")(Props{Class: "size-4 icon-test-cache"}).
		Render(t.Context(), &second))

	assert.Equal(t, first.String(), second.String(),
		"a cached glyph renders exactly what the generated one rendered")
}

func TestIconReportsAnUnknownName(t *testing.T) {
	resetIconCache(t)

	var buf strings.Builder

	err := Icon("definitely-not-an-icon")(Props{Class: "size-4"}).Render(t.Context(), &buf)

	require.Error(t, err)
	assert.ErrorContains(t, err, "definitely-not-an-icon")
	assert.ErrorContains(t, err, "not found in internalSvgData map")
	assert.ErrorContains(t, err, "failed to generate svg for icon",
		"the wrap says which render failed, not just that one did")
	assert.Empty(t, buf.String(), "nothing reaches the writer for an icon that does not exist")
	assert.Empty(t, iconContents, "a failed generation caches nothing")
}

func TestGetIconContentReturnsTheRawInnerMarkup(t *testing.T) {
	t.Parallel()

	content, err := getIconContent("check")
	require.NoError(t, err)
	assert.Equal(t, `<path d="M20 6 9 17l-5-5" />`, content,
		"the data map holds the inner markup, not a whole svg element")
	assert.NotContains(t, content, "<svg")
}

func TestGetIconContentReportsAMissingName(t *testing.T) {
	t.Parallel()

	content, err := getIconContent("definitely-not-an-icon")

	require.Error(t, err)
	assert.ErrorContains(t, err, "definitely-not-an-icon")
	assert.Empty(t, content)
}

func TestGenerateSVGWrapsTheGlyph(t *testing.T) {
	t.Parallel()

	svg, err := generateSVG("square", Props{Class: "size-4"})
	require.NoError(t, err)

	assert.Contains(t, svg, `<rect width="18" height="18" x="3" y="3" rx="2" />`)
	assert.Contains(t, svg, `stroke-width="2"`)
	assert.Contains(t, svg, `viewBox="0 0 24 24"`)
	assert.Contains(t, svg, `class="size-4"`)
}

// TestGenerateSVGInterpolatesTheClassWithoutEscaping documents that
// generateSVG formats props.Class straight into the class attribute. A class
// carrying a quote therefore closes the attribute early and the rest of the
// string is read by the parser as further attributes, so a caller that lets an
// untrusted value reach Props.Class can inject attributes onto the svg element.
// Every template that renders an icon passes a literal or a TwMerge result, so
// the value is trusted today; the escaping is what would close the hole.
func TestGenerateSVGInterpolatesTheClassWithoutEscaping(t *testing.T) {
	t.Parallel()

	svg, err := generateSVG("check", Props{Class: `x" onload="alert(1)`})
	require.NoError(t, err)

	assert.Contains(t, svg, `class="x" onload="alert(1)"`,
		"the quote is not escaped, so the attribute ends early")
	assert.NotContains(t, svg, "&#34;")
}

func TestEveryRegisteredIconHasGlyphContent(t *testing.T) {
	t.Parallel()

	names := registeredIconNames(t)
	require.NotEmpty(t, names)

	for _, name := range names {
		content, err := getIconContent(name)

		require.NoErrorf(
			t,
			err,
			"the registry binds %q but internalSvgData has no glyph for it",
			name,
		)
		assert.NotEmptyf(t, content, "the registry binds %q but its glyph is empty", name)
	}
}

func TestEveryGlyphIsReachableFromTheRegistry(t *testing.T) {
	t.Parallel()

	names := registeredIconNames(t)
	require.NotEmpty(t, names)

	registered := make(map[string]struct{}, len(names))
	for _, name := range names {
		registered[name] = struct{}{}
	}

	for name := range internalSvgData {
		assert.Containsf(t, registered, name,
			"internalSvgData carries %q but no registry var binds it", name)
	}
}

// registeredIconNames reads the generated registry and lists every icon name it
// binds.
//
// Parameters:
//   - t: Test whose failures report an unreadable registry.
//
// Returns:
//   - names: Every distinct icon name in the registry.
func registeredIconNames(t *testing.T) []string {
	t.Helper()

	source, err := os.ReadFile("icon_defs.go")
	require.NoError(t, err)

	matches := iconNamePattern.FindAllSubmatch(source, -1)
	require.NotEmpty(t, matches, "the generated registry no longer binds icon names")

	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))

	for _, match := range matches {
		name := string(match[1])
		if _, ok := seen[name]; ok {
			continue
		}

		seen[name] = struct{}{}

		names = append(names, name)
	}

	return names
}

// resetIconCache swaps in an empty glyph cache for the duration of one test.
//
// Parameters:
//   - t: Test whose cleanup restores the previous cache.
func resetIconCache(t *testing.T) {
	t.Helper()

	iconMutex.Lock()
	previous := iconContents
	iconContents = make(map[string]string)
	iconMutex.Unlock()

	t.Cleanup(func() {
		iconMutex.Lock()
		iconContents = previous
		iconMutex.Unlock()
	})
}

// cachedIcon reads one entry from the glyph cache.
//
// Parameters:
//   - t: Test whose failures report an unreadable cache.
//   - key: Cache key the icon component derives from a name and class.
//
// Returns:
//   - svg: The cached glyph, or an empty string when nothing is cached.
func cachedIcon(t *testing.T, key string) string {
	t.Helper()

	iconMutex.RLock()
	defer iconMutex.RUnlock()

	return iconContents[key]
}
