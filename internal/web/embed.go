// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package web assembles Outtake's inbound HTTP layer. It owns the route table,
// the middleware pipeline, and the embedded static assets, and it builds the
// handlers under web/handlers from the services the composition root hands it.
// The templ pages and components those handlers render live in web/pages,
// web/components, and web/view.
package web

import (
	"embed"
)

// Assets contains the embedded static assets (CSS, JavaScript, brand icons).
//
//go:embed assets
var Assets embed.FS
