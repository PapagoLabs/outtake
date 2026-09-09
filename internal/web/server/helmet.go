// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"github.com/gofiber/fiber/v3/middleware/helmet"
)

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self'; media-src 'self'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// HelmetConfig returns security headers including a same-origin CSP.
//
// Returns:
//   - cfg: The security headers including a same-origin CSP.
func HelmetConfig() helmet.Config {
	return helmet.Config{
		Next:                      nil,
		XSSProtection:             "0",
		ContentTypeNosniff:        "nosniff",
		XFrameOptions:             "DENY",
		ContentSecurityPolicy:     contentSecurityPolicy,
		ReferrerPolicy:            "no-referrer",
		PermissionPolicy:          "",
		CrossOriginEmbedderPolicy: "require-corp",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
		OriginAgentCluster:        "?1",
		XDNSPrefetchControl:       "off",
		XDownloadOptions:          "noopen",
		XPermittedCrossDomain:     "none",
		HSTSMaxAge:                0,
		HSTSExcludeSubdomains:     false,
		CSPReportOnly:             false,
		HSTSPreloadEnabled:        false,
	}
}
