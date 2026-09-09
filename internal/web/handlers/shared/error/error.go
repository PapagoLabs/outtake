// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package error

import (
	"errors"
	"fmt"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	pageerror "github.com/PapagoLabs/outtake/internal/web/pages/error"
)

// HttpErrorView is the status and copy for an HTML or JSON error response.
type HttpErrorView struct {
	code    int
	message string
	title   string
}

// PageError renders HTML error pages for browser requests and JSON for the API.
//
// Parameters:
//   - ctx: HTTP request context.
//   - err: Error value.
//
// Returns:
//   - err: The error, if any.
func PageError(ctx fiber.Ctx, err error) error {
	view := HttpErrorCopy(err)

	if respond.IsHTMXRequest(ctx) {
		err = respond.WriteHTMXFlash(ctx, view.code, view.message)
		if err != nil {
			return fmt.Errorf("write htmx flash: %w", err)
		}

		return nil
	}

	if strings.HasPrefix(ctx.Path(), "/api/") {
		return respond.WriteJSON(ctx, view.code, respond.ErrorResponse{
			Error:   "http_error",
			Message: view.message,
		})
	}

	ctx.Set(respond.HeaderContentType, respond.ContentTypeHTML)
	ctx.Status(view.code)

	err = pageerror.ErrorPage(pageerror.ErrorPageProps{
		Title:   view.title,
		Message: view.message,
		Status:  view.code,
	}).Render(ctx.Context(), ctx.Response().BodyWriter())
	if err != nil {
		return fmt.Errorf("render error page: %w", err)
	}

	return nil
}

// HttpErrorCopy maps an error onto status, message, and title.
//
// Parameters:
//   - err: Error value.
//
// Returns:
//   - httpErrorView: An error onto status, message, and title.
func HttpErrorCopy(err error) HttpErrorView {
	view := HttpErrorView{
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
