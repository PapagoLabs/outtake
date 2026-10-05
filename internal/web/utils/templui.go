// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package utils provides shared helpers for the Outtake web layer.
package utils

import (
	"context"
	"crypto/rand"
	"io"
	"strconv"
	"time"

	"github.com/a-h/templ"

	twmerge "github.com/Oudwins/tailwind-merge-go"
)

// ControlClass is the shared class list for text inputs and native selects.
const ControlClass = "flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs dark:bg-input/30 focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] outline-none aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40"

// TwMerge combines Tailwind classes and resolves conflicts.
//
// Parameters:
//   - classes: Class lists to combine. Later classes win a conflict.
//
// Returns:
//   - merged: The combined classes with conflicts resolved.
func TwMerge(classes ...string) string {
	return twmerge.Merge(classes...)
}

// If returns value when condition is true and the zero value of T otherwise.
//
// Parameters:
//   - condition: Whether to return value.
//   - value: The value to return when condition is true.
//
// Returns:
//   - result: value when condition is true, otherwise the zero value of T.
func If[T any](condition bool, value T) T {
	var empty T

	if condition {
		return value
	}

	return empty
}

// IfElse returns one of two values based on condition.
//
// Parameters:
//   - condition: Whether to return trueValue.
//   - trueValue: The value to return when condition is true.
//   - falseValue: The value to return when condition is false.
//
// Returns:
//   - result: trueValue when condition is true, otherwise falseValue.
func IfElse[T any](condition bool, trueValue, falseValue T) T {
	if condition {
		return trueValue
	}

	return falseValue
}

// RandomID generates a random ID string.
//
// Returns:
//   - id: The generated identifier, prefixed with "id-".
func RandomID() string {
	return "id-" + rand.Text()
}

// ScriptVersion is a timestamp generated at app start for cache busting.
var ScriptVersion = strconv.FormatInt(time.Now().Unix(), 10)

// ScriptURL appends the cache-busting query to a script path.
var ScriptURL = func(path string) string {
	return path + "?v=" + ScriptVersion
}

// componentScriptBasePath is the base public path for component JavaScript files.
var componentScriptBasePath = "/assets/js"

// UseUnminifiedScripts switches component script loading to the unminified files.
var UseUnminifiedScripts = false

// ComponentScript renders a deferred script tag for a component JavaScript file.
//
// Parameters:
//   - component: Component name whose script is loaded.
//
// Returns:
//   - script: A component rendering the script tag.
func ComponentScript(component string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		nonce := templ.GetNonce(ctx)
		fileName := component + ".min.js"
		if UseUnminifiedScripts {
			fileName = component + ".js"
		}

		src := ScriptURL(componentScriptBasePath + "/" + fileName)

		if _, err := io.WriteString(w, `<script type="module"`); err != nil {
			return err
		}
		if nonce != "" {
			if _, err := io.WriteString(w, ` nonce="`); err != nil {
				return err
			}
			if _, err := io.WriteString(w, templ.EscapeString(nonce)); err != nil {
				return err
			}
			if _, err := io.WriteString(w, `"`); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, ` src="`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, templ.EscapeString(src)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `"></script>`); err != nil {
			return err
		}

		return nil
	})
}
