// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package icon

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/a-h/templ"
)

// iconContents caches the fully generated SVG strings for icons that have been used,
// keyed by a composite key of name and props to handle different stylings.
var (
	iconContents = make(map[string]string)
	iconMutex    sync.RWMutex
)

// Props defines the properties that can be set for an icon.
type Props struct {
	Class string
}

// Icon returns a function that generates a templ.Component for the specified
// icon name.
//
// Parameters:
//   - name: Icon identifier from the registered set.
//
// Returns:
//   - func: A function that generates a templ.Component for the specified icon.
func Icon(name string) func(...Props) templ.Component {
	return func(props ...Props) templ.Component {
		var p Props

		if len(props) > 0 {
			p = props[0]
		}

		// Cache by icon name and class so repeated renders reuse the generated SVG.
		cacheKey := fmt.Sprintf("%s|cl:%s", name, p.Class)

		return templ.ComponentFunc(func(ctx context.Context, w io.Writer) (err error) {
			iconMutex.RLock()

			svg, cached := iconContents[cacheKey]

			iconMutex.RUnlock()

			if cached {
				_, err = w.Write([]byte(svg))

				return err
			}

			// Not cached, generate it
			// The actual generation now happens once and is cached.
			generatedSvg, err := generateSVG(name, p) // p (Props) is passed to generateSVG
			if err != nil {
				// Provide more context in the error message
				return fmt.Errorf(
					"failed to generate svg for icon '%s' with props %+v: %w",
					name,
					p,
					err,
				)
			}

			iconMutex.Lock()

			iconContents[cacheKey] = generatedSvg
			iconMutex.Unlock()

			_, err = w.Write([]byte(generatedSvg))

			return err
		})
	}
}

// generateSVG creates an SVG string for the specified icon with the given
// properties.
//
// Parameters:
//   - name: Icon identifier from the registered set.
//   - props: Component property bag.
//
// Returns:
//   - value: An SVG string for the specified icon with the given properties.
//   - err: Non-nil when the call fails.
func generateSVG(name string, props Props) (string, error) {
	// Get the raw, inner SVG content for the icon name from our internal data map.
	content, err := getIconContent(name) // This now reads from internalSvgData
	if err != nil {
		return "", err // Error from getIconContent already includes icon name
	}

	// Construct the final SVG string.
	// The data-lucide attribute helps identify these as Lucide icons if needed.
	return fmt.Sprintf(
		"<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"24\" height=\"24\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\" stroke-linejoin=\"round\" class=\"%s\" data-lucide=\"icon\">%s</svg>",
		props.Class,
		content,
	), nil
}

// getIconContent retrieves the raw inner SVG content for a given icon name.
//
// Parameters:
//   - name: Icon identifier from the registered set.
//
// Returns:
//   - value: Result value. Zero or empty when unavailable.
//   - err: Non-nil when name is not a registered icon.
func getIconContent(name string) (string, error) {
	content, exists := internalSvgData[name]
	if !exists {
		return "", fmt.Errorf("icon '%s' not found in internalSvgData map", name)
	}

	return content, nil
}
