// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package respond

import (
	"errors"
	"fmt"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// httpErrorView is the status and copy for an HTML or JSON error response.
type httpErrorView struct {
	code    int
	message string
	title   string
}

// PageError renders HTML error pages for browser requests and JSON for the API.
//
// Parameters:
//   - ctx: Request that failed.
//   - err: Error raised by the handler.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func PageError(ctx fiber.Ctx, err error) error {
	page := httpErrorCopy(err)
	shown := pageFailure(ctx, page, err)

	if IsHTMXRequest(ctx) {
		err = WriteHTMXFlash(ctx, page.code, shown)
		if err != nil {
			return fmt.Errorf("write htmx flash: %w", err)
		}

		return nil
	}

	if strings.HasPrefix(ctx.Path(), "/api/") {
		return WriteJSON(ctx, page.code, api.ErrorResponse{
			Error:   api.HTTPError,
			Message: shown.Message,
			Details: shown.Details,
		})
	}

	ctx.Set(fiber.HeaderContentType, contentTypeHTML)
	ctx.Status(page.code)

	err = pages.ErrorPage(pages.ErrorPageProps{
		Title:   page.title,
		Message: shown.Message,
		Details: shown.Details,
		Status:  page.code,
	}).Render(ctx.Context(), ctx.Response().BodyWriter())
	if err != nil {
		return fmt.Errorf("render error page: %w", err)
	}

	return nil
}

// pageFailure is what an error page or banner shows: details for an
// unexpected failure, and the message alone for a missing page or a refusal
// Fiber reports with its own status, such as an expired form.
//
// Parameters:
//   - ctx: Request that failed.
//   - page: The status and copy httpErrorCopy chose.
//   - err: Error raised by the handler.
//
// Returns:
//   - failure: The message, with details for an unexpected failure.
func pageFailure(ctx fiber.Ctx, page httpErrorView, err error) view.Failure {
	if page.code != fiber.StatusInternalServerError {
		return view.NewNotice(page.message)
	}

	return FailWith(ctx, page.message, err)
}

// httpErrorCopy maps an error onto status, message, and title.
//
// Parameters:
//   - err: Error raised by the handler.
//
// Returns:
//   - page: Status code and copy for the error response.
func httpErrorCopy(err error) httpErrorView {
	page := httpErrorView{
		code:    fiber.StatusInternalServerError,
		message: "Something went wrong. Check the Outtake log.",
		title:   "Something Went Wrong",
	}

	if ferr, ok := errors.AsType[*fiber.Error](err); ok {
		page.code = ferr.Code
		if ferr.Message != "" && ferr.Code != fiber.StatusInternalServerError {
			page.message = ferr.Message
		}
	}

	if page.code == fiber.StatusNotFound {
		page.message = "That page doesn't exist"
		page.title = "Page Not Found"
	}

	return page
}
