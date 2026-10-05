// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package respond holds how a handler answers a request.
package respond

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/web/components/flash"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

const (
	// contentTypeHTML is the content type written by page handlers.
	contentTypeHTML = "text/html; charset=utf-8"

	// formContentType is the content type a browser form post carries.
	formContentType = "application/x-www-form-urlencoded"

	// hxRequestValue is the HX-Request header value htmx sends.
	hxRequestValue = "true"

	// formFieldMediaID is the form field naming the source a clip is cut from.
	formFieldMediaID = "mediaId"
)

// NotFoundMessage is the copy shown for a resource nothing is registered under.
const NotFoundMessage = "clip not found"

// WriteJSON writes a JSON response and wraps Fiber errors.
//
// Parameters:
//   - ctx: Request context.
//   - status: HTTP status code.
//   - payload: Value encoded as the response body.
//
// Returns:
//   - err: Wrapped write error, or nil on success.
func WriteJSON(ctx fiber.Ctx, status int, payload any) error {
	err := ctx.Status(status).JSON(payload)
	if err != nil {
		return fmt.Errorf("write json: %w", err)
	}

	return nil
}

// SendRangedFile serves a media file with HTTP byte-range support.
//
// Parameters:
//   - ctx: Request context.
//   - path: Path of the file to serve.
//
// Returns:
//   - err: Wrapped send error, or nil on success.
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
func WriteError(ctx fiber.Ctx, status int, code api.ErrorCode, message string) error {
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

// WriteNotFound reports a resource nothing is registered under.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - Wrapped write or redirect error.
func WriteNotFound(ctx fiber.Ctx) error {
	return WriteError(ctx, fiber.StatusNotFound, api.NotFound, NotFoundMessage)
}

// WriteHTMXFlash writes an HTMX partial error banner for #flash.
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
	// A browser control such as the delete button swaps on any status other
	// than 204 or 304, so a failure would remove the card. A JSON caller that
	// also set HX-Request is driving the swap itself through the partial.
	if !isJSONRequest(ctx) {
		ctx.Set(routes.HeaderHXReswap, routes.HXTargetNone)
	}

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
	return ctx.Get(routes.HeaderHXRequest) == hxRequestValue
}

// isJSONRequest reports whether the body is JSON.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - True when the Content-Type says JSON.
func isJSONRequest(ctx fiber.Ctx) bool {
	return strings.Contains(ctx.Get(fiber.HeaderContentType), "json")
}

// IsFormRequest reports whether the request is urlencoded form data.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - True when the Content-Type is urlencoded form data.
func IsFormRequest(ctx fiber.Ctx) bool {
	return strings.Contains(ctx.Get(fiber.HeaderContentType), formContentType)
}

// HXTargetID returns the element id from an HTMX 4 HX-Target header.
//
// Parameters:
//   - raw: Header value, either an id or tag#id.
//
// Returns:
//   - Element id, or the raw value when no hash is present.
func HXTargetID(raw string) string {
	_, id, found := strings.Cut(raw, "#")
	if found {
		return id
	}

	return raw
}

// TargetsElement reports whether the request asks to swap one element id.
//
// Parameters:
//   - ctx: Request context, whose HX-Target header names the element.
//   - id: Element id the caller renders for.
//
// Returns:
//   - True when HTMX is targeting that element.
func TargetsElement(ctx fiber.Ctx, id string) bool {
	return HXTargetID(ctx.Get(routes.HeaderHXTarget)) == id
}

// FormInt reads an integer form field, treating a bad value as unset.
//
// Parameters:
//   - ctx: Request context.
//   - name: Form field to read.
//
// Returns:
//   - value: The parsed integer, or zero when the field is absent or malformed.
func FormInt(ctx fiber.Ctx, name string) int {
	return atoiOrZero(ctx.FormValue(name))
}

// QueryInt reads an integer query parameter, treating a bad value as unset.
//
// Parameters:
//   - ctx: Request context.
//   - name: Query parameter to read.
//
// Returns:
//   - value: The parsed integer, or zero when the parameter is absent or
//     malformed.
func QueryInt(ctx fiber.Ctx, name string) int {
	return atoiOrZero(ctx.Query(name))
}

// atoiOrZero parses an integer, treating a malformed value as zero.
//
// Parameters:
//   - raw: Value to parse.
//
// Returns:
//   - value: The parsed integer, or zero when it is not a number.
func atoiOrZero(raw string) int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}

	return value
}

// QueryValue reads one query parameter out of a URL.
//
// Parameters:
//   - raw: URL to read, which may be relative.
//   - name: Query parameter to read.
//
// Returns:
//   - value: The parameter value, or an empty string when absent or unparseable.
func QueryValue(raw, name string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return parsed.Query().Get(name)
}

// FormErrorLocation returns the HTML page that should show a form error.
//
// Parameters:
//   - ctx: Request context.
//   - message: Flash text carried in the redirect query.
//
// Returns:
//   - location: A path-only location carrying the encoded error.
func FormErrorLocation(ctx fiber.Ctx, message string) string {
	referer := ctx.Get(fiber.HeaderReferer)
	if referer != "" {
		return PathWithError(RefererPath(referer), message)
	}

	if mediaID := ctx.FormValue(formFieldMediaID); mediaID != "" {
		return PathWithError(routes.ItemURL(mediaID, nil), message)
	}

	return PathWithError(routes.PathRoot, message)
}

// RefererOrFallback prefers the page a form was submitted from.
//
// Parameters:
//   - ctx: Request context.
//   - fallback: Path used when the request carries no Referer.
//
// Returns:
//   - location: A path-only redirect target.
func RefererOrFallback(ctx fiber.Ctx, fallback string) string {
	referer := ctx.Get(fiber.HeaderReferer)
	if referer != "" {
		return RefererPath(referer)
	}

	return fallback
}

// PathWithError appends an encoded error query to a path-only location.
//
// Parameters:
//   - location: Path-only redirect target.
//   - message: Flash text to carry in the query.
//
// Returns:
//   - location: The location with the encoded error query, or the dashboard when
//     the input cannot be parsed.
func PathWithError(location, message string) string {
	parsed, err := url.Parse(location)
	if err != nil || parsed.Path == "" {
		parsed, err = url.Parse(routes.PathRoot)
		if err != nil {
			return routes.PathRoot
		}
	}

	query := parsed.Query()
	query.Set(routes.QueryError, message)

	return parsed.Path + "?" + query.Encode()
}

// RefererPath keeps only the path and query of a Referer URL.
//
// Parameters:
//   - raw: Referer header value.
//
// Returns:
//   - path: The path and query, or the dashboard when the value cannot be parsed.
func RefererPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" {
		return routes.PathRoot
	}

	if parsed.RawQuery == "" {
		return parsed.Path
	}

	return parsed.Path + "?" + parsed.RawQuery
}

// RedirectTo issues a redirect and wraps Fiber errors.
//
// Parameters:
//   - ctx: Request context.
//   - location: Redirect target.
//
// Returns:
//   - err: Wrapped redirect error, or nil on success.
func RedirectTo(ctx fiber.Ctx, location string) error {
	err := ctx.Redirect().To(location)
	if err != nil {
		return fmt.Errorf("redirect: %w", err)
	}

	return nil
}

// SendText writes a plain-text body and wraps Fiber errors.
//
// Parameters:
//   - ctx: Request context.
//   - body: Response body.
//
// Returns:
//   - err: Wrapped write error, or nil on success.
func SendText(ctx fiber.Ctx, body string) error {
	err := ctx.SendString(body)
	if err != nil {
		return fmt.Errorf("send string: %w", err)
	}

	return nil
}

// SendStatusCode writes a status with no body and wraps Fiber errors.
//
// Parameters:
//   - ctx: Request context.
//   - status: HTTP status code.
//
// Returns:
//   - err: Wrapped write error, or nil on success.
func SendStatusCode(ctx fiber.Ctx, status int) error {
	err := ctx.SendStatus(status)
	if err != nil {
		return fmt.Errorf("send status: %w", err)
	}

	return nil
}

// RenderHTML writes a templ component and wraps render errors.
//
// Parameters:
//   - ctx: Request context, whose Content-Type header is set to HTML.
//   - render: Component render function writing to the response body.
//
// Returns:
//   - err: Wrapped render error, or nil on success.
func RenderHTML(ctx fiber.Ctx, render func(w io.Writer) error) error {
	ctx.Set(fiber.HeaderContentType, contentTypeHTML)

	err := render(ctx.Response().BodyWriter())
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}

	return nil
}
