// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"context"
	"net/url"

	fiberClient "github.com/gofiber/fiber/v3/client"
)

// newURL builds a [*url.URL] with every field set.
func newURL(scheme, host, path, rawQuery string) *url.URL {
	return &url.URL{
		Scheme:      scheme,
		Opaque:      "",
		User:        nil,
		Host:        host,
		Path:        path,
		RawPath:     "",
		OmitHost:    false,
		ForceQuery:  false,
		RawQuery:    rawQuery,
		Fragment:    "",
		RawFragment: "",
	}
}

// newRequestConfig builds a fully populated Fiber client request config.
func newRequestConfig(ctx context.Context, headers map[string]string, body any) fiberClient.Config {
	return fiberClient.Config{
		Ctx:                    ctx,
		Body:                   body,
		Header:                 headers,
		Param:                  map[string]string{},
		Cookie:                 map[string]string{},
		PathParam:              map[string]string{},
		FormData:               map[string]string{},
		UserAgent:              "",
		Referer:                "",
		File:                   nil,
		Timeout:                0,
		MaxRedirects:           0,
		DisablePathNormalizing: false,
	}
}

// defaultBaseURL returns the plex.tv API origin.
func defaultBaseURL() *url.URL {
	return newURL(defaultScheme, defaultHost, "", "")
}
