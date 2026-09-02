// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"fmt"
	"io"
	"net/url"

	"github.com/gofiber/fiber/v3/middleware/session"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
)

const (
	// HeaderContentType is the HTTP Content-Type header name.
	headerContentType = "Content-Type"

	// PathRoot is the dashboard path.
	pathRoot = "/"

	// PathLogin is the login page path.
	pathLogin = "/login"

	// PathClips is the clips page path.
	pathClips = "/clips"

	// PathServers is the server picker path.
	pathServers = "/servers"

	// PathSettingsProfiles is the clip profile settings path.
	pathSettingsProfiles = "/settings/profiles"

	// QueryTitle is the media browse title query parameter.
	queryTitle = "title"

	// QueryError is the flash-error query parameter on HTML pages.
	queryError = "error"

	// DefaultSegmentSecs is the fallback clip window when end is omitted.
	defaultSegmentSecs = 10

	// InvalidRequest is the API error code for a malformed clip request.
	invalidRequest = "invalid_request"

	// PersistFailed is the API error code when a clip cannot be saved.
	persistFailed = "persist_failed"

	// FloatBitSize is the bit size used when parsing floats.
	floatBitSize = 64

	// DefaultMaxClipDur is the fallback maximum clip duration in seconds.
	defaultMaxClipDur = 600

	// ContentTypeHTML is the HTML content type written by page handlers.
	contentTypeHTML = "text/html; charset=utf-8"

	// MediaLoadFailedMsg is shown when Plex metadata cannot be loaded.
	mediaLoadFailedMsg = "Could not load this item from Plex. You can still create a clip if the file is reachable."
)

// writeJSON writes a JSON response and wraps Fiber errors.
func writeJSON(ctx fiber.Ctx, status int, payload any) error {
	err := ctx.Status(status).JSON(payload)
	if err != nil {
		return fmt.Errorf("write json: %w", err)
	}

	return nil
}

// writeError writes a JSON error payload, or redirects HTML form posts.
func writeError(ctx fiber.Ctx, status int, code, message string) error {
	if isFormRequest(ctx) {
		return redirectTo(ctx, formErrorLocation(ctx, message))
	}

	return writeJSON(ctx, status, api.ErrorResponse{
		Error:   code,
		Message: message,
	})
}

// formErrorLocation returns the HTML page that should show a form error.
func formErrorLocation(ctx fiber.Ctx, message string) string {
	mediaID := ctx.FormValue("mediaId")
	if mediaID != "" {
		return pathWithError(clipReturnPath(mediaID), message)
	}

	referer := ctx.Get(fiber.HeaderReferer)
	if referer != "" {
		return pathWithError(refererPath(referer), message)
	}

	return pathWithError(pathRoot, message)
}

// pathWithError appends an encoded error query to a path-only location.
func pathWithError(location, message string) string {
	parsed, err := url.Parse(location)
	if err != nil || parsed.Path == "" {
		location = pathRoot
		parsed, err = url.Parse(location)
		if err != nil {
			return pathRoot
		}
	}

	query := parsed.Query()
	query.Set(queryError, message)

	return parsed.Path + "?" + query.Encode()
}

// refererPath keeps only the path and query of a Referer URL.
func refererPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" {
		return pathRoot
	}

	if parsed.RawQuery == "" {
		return parsed.Path
	}

	return parsed.Path + "?" + parsed.RawQuery
}

// mediaItemError prefers a form-flash query over a Plex metadata load failure.
func mediaItemError(itemErr error, queryErr string) string {
	if queryErr != "" {
		return queryErr
	}

	if itemErr != nil {
		return mediaLoadFailedMsg
	}

	return ""
}

// redirectTo issues a redirect and wraps Fiber errors.
func redirectTo(ctx fiber.Ctx, location string) error {
	err := ctx.Redirect().To(location)
	if err != nil {
		return fmt.Errorf("redirect: %w", err)
	}

	return nil
}

// sendText writes a plain-text body and wraps Fiber errors.
func sendText(ctx fiber.Ctx, body string) error {
	err := ctx.SendString(body)
	if err != nil {
		return fmt.Errorf("send string: %w", err)
	}

	return nil
}

// sendStatusCode writes a status with no body and wraps Fiber errors.
func sendStatusCode(ctx fiber.Ctx, status int) error {
	err := ctx.SendStatus(status)
	if err != nil {
		return fmt.Errorf("send status: %w", err)
	}

	return nil
}

// renderHTML writes a templ component and wraps render errors.
func renderHTML(ctx fiber.Ctx, render func(w io.Writer) error) error {
	ctx.Set(headerContentType, contentTypeHTML)

	err := render(ctx.Response().BodyWriter())
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}

	return nil
}

// sessionString reads a string value from the Fiber session.
func sessionString(sess *session.Middleware, key string) string {
	if sess == nil {
		return ""
	}

	value, ok := sess.Get(key).(string)
	if !ok {
		return ""
	}

	return value
}

// sessionInt reads an int value from the Fiber session.
func sessionInt(sess *session.Middleware, key string) int {
	if sess == nil {
		return 0
	}

	value, ok := sess.Get(key).(int)
	if !ok {
		return 0
	}

	return value
}
