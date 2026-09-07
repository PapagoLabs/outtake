// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"errors"
	"fmt"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/web/pages"
)

// httpErrorView is the status and copy for an HTML or JSON error response.
type httpErrorView struct {
	code    int
	message string
	title   string
}

// PageError renders HTML error pages for browser requests and JSON for the API.
func PageError(ctx fiber.Ctx, err error) error {
	view := httpErrorCopy(err)

	if isHTMXRequest(ctx) {
		err = writeHTMXFlash(ctx, view.code, view.message)
		if err != nil {
			return fmt.Errorf("write htmx flash: %w", err)
		}

		return nil
	}

	if strings.HasPrefix(ctx.Path(), "/api/") {
		return writeJSON(ctx, view.code, api.ErrorResponse{
			Error:   "http_error",
			Message: view.message,
		})
	}

	ctx.Set(headerContentType, contentTypeHTML)
	ctx.Status(view.code)

	err = pages.ErrorPage(pages.ErrorPageProps{
		Title:   view.title,
		Message: view.message,
		Status:  view.code,
	}).Render(ctx.Context(), ctx.Response().BodyWriter())
	if err != nil {
		return fmt.Errorf("render error page: %w", err)
	}

	return nil
}

// httpErrorCopy maps an error onto status, message, and title.
func httpErrorCopy(err error) httpErrorView {
	view := httpErrorView{
		code:    fiber.StatusInternalServerError,
		message: "Something went wrong.",
		title:   "Something went wrong",
	}

	if ferr, ok := errors.AsType[*fiber.Error](err); ok {
		view.code = ferr.Code
		if ferr.Message != "" && ferr.Code != fiber.StatusInternalServerError {
			view.message = ferr.Message
		}
	}

	if view.code == fiber.StatusNotFound {
		view.message = "That page does not exist."
		view.title = "Page not found"
	}

	return view
}
