// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package aspectratio

import (
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderAspectRatio(t *testing.T, props ...Props) string {
	t.Helper()

	var buf strings.Builder

	err := AspectRatio(props...).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

func TestAspectRatioDefaultsToAnUnsetRatio(t *testing.T) {
	t.Parallel()

	body := renderAspectRatio(t)

	assert.Subset(t,
		strings.Fields(regexp.MustCompile(`class="([^"]*)"`).FindStringSubmatch(body)[1]),
		[]string{"relative", "w-full", "aspect-auto"},
	)
	assert.NotContains(t, body, `id=`, "an absent id leaves the attribute off")
}

func TestAspectRatioKeepsTheRatioClass(t *testing.T) {
	t.Parallel()

	tests := map[Ratio]string{
		RatioAuto:         "aspect-auto",
		RatioSquare:       "aspect-square",
		RatioVideo:        "aspect-video",
		RatioPortrait:     "aspect-[3/4]",
		RatioWide:         "aspect-[2/1]",
		Ratio("nonsense"): "aspect-auto",
		Ratio(""):         "aspect-auto",
	}

	for ratio, want := range tests {
		t.Run(string(ratio), func(t *testing.T) {
			t.Parallel()

			body := renderAspectRatio(t, Props{Ratio: ratio})

			assert.Contains(t, body, want)
			assert.Contains(t, body, `<div class="absolute inset-0">`)
		})
	}
}

func TestAspectRatioRendersIdentityAndClass(t *testing.T) {
	t.Parallel()

	body := renderAspectRatio(t, Props{
		ID:    "preview-frame",
		Ratio: RatioVideo,
		Class: "rounded-md",
		Attributes: templ.Attributes{
			"data-testid": "frame",
		},
	})

	assert.Contains(t, body, `id="preview-frame"`)
	assert.Contains(t, body, "aspect-video")
	assert.Contains(t, body, "rounded-md")
	assert.Contains(t, body, `data-testid="frame"`)
}
