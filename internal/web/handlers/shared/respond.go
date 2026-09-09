// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3/middleware/session"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/api"
	"github.com/PapagoLabs/outtake/internal/web/components/flash"
)

const (
	// HeaderContentType is the HTTP Content-Type header name.
	HeaderContentType = "Content-Type"

	// PathRoot is the dashboard path.
	PathRoot = "/"

	// PathLogin is the login page path.
	PathLogin = "/login"

	// PathClips is the clips page path.
	PathClips = "/clips"

	// PathServers is the server picker path.
	PathServers = "/servers"

	// PathSettingsProfiles is the clip profile settings path.
	PathSettingsProfiles = "/settings/profiles"

	// PathMedia is the media library path.
	PathMedia = "/media"

	// QueryTitle is the media browse title query parameter.
	QueryTitle = "title"

	// QueryLibrary is the media library id query parameter.
	QueryLibrary = "library"

	// QueryStart is the media pagination offset.
	QueryStart = "start"

	// QueryParent is the media container parent id.
	QueryParent = "parent"

	// QueryUp is the media breadcrumb parent id.
	QueryUp = "up"

	// QueryUpTitle is the media breadcrumb parent title.
	QueryUpTitle = "upTitle"

	// QueryError is the flash-error query parameter on HTML pages.
	QueryError = "error"

	// QueryWebSafeColor carries the New export web-safe color checkbox.
	QueryWebSafeColor = "webSafeColor"

	// QueryUnchecked is the query value for an explicit false checkbox.
	QueryUnchecked = "0"

	// DefaultSegmentSecs is the fallback clip window when end is omitted.
	DefaultSegmentSecs = 10

	// InvalidRequest is the API error code for a malformed clip request.
	InvalidRequest = "invalid_request"

	// PersistFailed is the API error code when a clip cannot be saved.
	PersistFailed = "persist_failed"

	// FloatBitSize is the bit size used when parsing floats.
	FloatBitSize = 64

	// DefaultMaxClipDur is the fallback maximum clip duration in seconds.
	DefaultMaxClipDur = 600

	// MediaPageSize is the number of posters shown per media library page.
	MediaPageSize = 48

	// ContentTypeHTML is the HTML content type written by page handlers.
	ContentTypeHTML = "text/html; charset=utf-8"

	// HeaderHXRequest is the HTMX request marker header.
	HeaderHXRequest = "HX-Request"

	// HeaderHXTarget is the HTMX swap-target header.
	HeaderHXTarget = "HX-Target"

	// MediaLoadFailedMsg is shown when Plex metadata cannot be loaded.
	MediaLoadFailedMsg = "Could not load this item from Plex. You can still create a clip if the file is reachable."
)

// WriteJSON writes a JSON response and wraps Fiber errors.
func WriteJSON(ctx fiber.Ctx, status int, payload any) error {
	err := ctx.Status(status).JSON(payload)
	if err != nil {
		return fmt.Errorf("write json: %w", err)
	}

	return nil
}

// SendRangedFile serves a media file with HTTP byte-range support.
func SendRangedFile(ctx fiber.Ctx, path string) error {
	err := ctx.SendFile(path, fiber.SendFile{
		FS:            nil,
		Compress:      false,
		ByteRange:     true,
		Download:      false,
		CacheDuration: 0,
		MaxAge:        0,
	})
	if err != nil {
		return fmt.Errorf("send ranged file: %w", err)
	}

	return nil
}

// WriteError writes a JSON error payload, or redirects HTML form posts.
//
// Parameters:
//   - ctx: Request context.
//   - status: HTTP status code.
//   - code: Machine-readable API error code.
//   - message: Human-readable error text.
//
// Returns:
//   - Wrapped write or redirect error.
func WriteError(ctx fiber.Ctx, status int, code, message string) error {
	if IsHTMXRequest(ctx) {
		err := WriteHTMXFlash(ctx, status, message)
		if err != nil {
			return fmt.Errorf("write htmx flash: %w", err)
		}

		return nil
	}

	if IsFormRequest(ctx) {
		return RedirectTo(ctx, FormErrorLocation(ctx, message))
	}

	return WriteJSON(ctx, status, api.ErrorResponse{
		Error:   code,
		Message: message,
	})
}

// WriteHTMXFlash writes a pure hx-partial error banner for #flash.
//
// Parameters:
//   - ctx: Request context.
//   - status: HTTP status code.
//   - message: Flash text.
//
// Returns:
//   - Wrapped render error.
func WriteHTMXFlash(ctx fiber.Ctx, status int, message string) error {
	ctx.Status(status)

	return RenderHTML(ctx, func(writer io.Writer) error {
		return flash.Partial(message).Render(ctx.Context(), writer)
	})
}

// IsHTMXRequest reports whether the client sent HX-Request.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - True when HTMX issued the request.
func IsHTMXRequest(ctx fiber.Ctx) bool {
	return ctx.Get(HeaderHXRequest) == "true"
}

// HxTargetID returns the element id from an HTMX 4 HX-Target header.
//
// Parameters:
//   - raw: Header value, either an id or tag#id.
//
// Returns:
//   - Element id, or the raw value when no hash is present.
func HxTargetID(raw string) string {
	_, id, found := strings.Cut(raw, "#")
	if found {
		return id
	}

	return raw
}

// FormErrorLocation returns the HTML page that should show a form error.
func FormErrorLocation(ctx fiber.Ctx, message string) string {
	mediaID := ctx.FormValue("mediaId")
	if mediaID != "" {
		return PathWithError(ClipReturnPath(mediaID), message)
	}

	referer := ctx.Get(fiber.HeaderReferer)
	if referer != "" {
		return PathWithError(RefererPath(referer), message)
	}

	return PathWithError(PathRoot, message)
}

// PathWithError appends an encoded error query to a path-only location.
func PathWithError(location, message string) string {
	parsed, err := url.Parse(location)
	if err != nil || parsed.Path == "" {
		location = PathRoot
		parsed, err = url.Parse(location)
		if err != nil {
			return PathRoot
		}
	}

	query := parsed.Query()
	query.Set(QueryError, message)

	return parsed.Path + "?" + query.Encode()
}

// RefererPath keeps only the path and query of a Referer URL.
func RefererPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" {
		return PathRoot
	}

	if parsed.RawQuery == "" {
		return parsed.Path
	}

	return parsed.Path + "?" + parsed.RawQuery
}

// MediaItemError prefers a form-flash query over a Plex metadata load failure.
func MediaItemError(itemErr error, queryErr string) string {
	if queryErr != "" {
		return queryErr
	}

	if itemErr != nil {
		return MediaLoadFailedMsg
	}

	return ""
}

// RedirectTo issues a redirect and wraps Fiber errors.
func RedirectTo(ctx fiber.Ctx, location string) error {
	err := ctx.Redirect().To(location)
	if err != nil {
		return fmt.Errorf("redirect: %w", err)
	}

	return nil
}

// SendText writes a plain-text body and wraps Fiber errors.
func SendText(ctx fiber.Ctx, body string) error {
	err := ctx.SendString(body)
	if err != nil {
		return fmt.Errorf("send string: %w", err)
	}

	return nil
}

// SendStatusCode writes a status with no body and wraps Fiber errors.
func SendStatusCode(ctx fiber.Ctx, status int) error {
	err := ctx.SendStatus(status)
	if err != nil {
		return fmt.Errorf("send status: %w", err)
	}

	return nil
}

// RenderHTML writes a templ component and wraps render errors.
func RenderHTML(ctx fiber.Ctx, render func(w io.Writer) error) error {
	ctx.Set(HeaderContentType, ContentTypeHTML)

	err := render(ctx.Response().BodyWriter())
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}

	return nil
}

// SessionString reads a string value from the Fiber session.
func SessionString(sess *session.Middleware, key string) string {
	if sess == nil {
		return ""
	}

	value, ok := sess.Get(key).(string)
	if !ok {
		return ""
	}

	return value
}

// SessionInt reads an int value from the Fiber session.
func SessionInt(sess *session.Middleware, key string) int {
	if sess == nil {
		return 0
	}

	value, ok := sess.Get(key).(int)
	if !ok {
		return 0
	}

	return value
}

// IsFormRequest reports whether the request is urlencoded form data.
func IsFormRequest(ctx fiber.Ctx) bool {
	return strings.Contains(ctx.Get(fiber.HeaderContentType), "application/x-www-form-urlencoded")
}

// ClipReturnPath sends form posts back to the source media item when possible.
func ClipReturnPath(mediaID string) string {
	if mediaID == "" {
		return PathClips
	}

	return PathMedia + "/item/" + mediaID
}
