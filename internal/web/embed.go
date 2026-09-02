// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package web provides embedded static assets and templ-based UI components
// and pages for Outtake.
package web

import (
	"embed"
)

// Assets contains the embedded static assets (CSS, JavaScript, brand icons).
//
//go:embed assets
var Assets embed.FS
