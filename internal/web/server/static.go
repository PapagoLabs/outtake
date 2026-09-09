// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/PapagoLabs/outtake/internal/web"
)

// StaticConfig returns the static asset middleware configuration.
func StaticConfig() static.Config {
	return static.Config{
		FS:              web.Assets,
		Next:            nil,
		ModifyResponse:  nil,
		NotFoundHandler: nil,
		IndexNames:      []string{"index.html"},
		CacheDuration:   0,
		MaxAge:          0,
		Compress:        false,
		ByteRange:       false,
		Browse:          false,
		Download:        false,
	}
}
