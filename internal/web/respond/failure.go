// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package respond

import (
	"strings"

	"github.com/gofiber/fiber/v3/middleware/session"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/failure"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

const (
	// sessionKeyFlashMessage holds a failure's message across a redirect.
	sessionKeyFlashMessage = "flash_message"
	// sessionKeyFlashDetails holds a failure's details across a redirect.
	sessionKeyFlashDetails = "flash_details"
	// minTokenLength is the shortest session token scrubbed by value, so a
	// short or empty value never blanks ordinary text.
	minTokenLength = 8
)

// Fail builds the failure a page shows for err, with the message
// failure.Classify chooses. A refusal of what the user entered shows its message alone. Any
// other failure carries details and is logged under the reference they hold.
//
// Parameters:
//   - ctx: The request that failed.
//   - err: What went wrong.
//
// Returns:
//   - failure: The plain message, with details unless it refuses input.
func Fail(ctx fiber.Ctx, err error) view.Failure {
	message, input := failure.Classify(err)
	if input {
		return view.NewNotice(message)
	}

	return FailWith(ctx, message, err)
}

// FailWith builds the failure a page shows for err under a message the caller
// chose, and logs it under the reference its details carry. The details and
// the log line carry no Plex token. Only a signed-in session sees the
// details: anyone else gets the message alone, so an error chain never
// reaches a visitor who is not the owner.
//
// Parameters:
//   - ctx: The request that failed.
//   - message: The plain message.
//   - err: What went wrong, nil for a refusal the message explains in full.
//
// Returns:
//   - failure: The message, with details when err is set and the session is
//     signed in.
func FailWith(ctx fiber.Ctx, message string, err error) view.Failure {
	if err == nil {
		return view.NewNotice(message)
	}

	ref := failure.NewRef()
	chain := scrub(ctx, err.Error())

	logging.Logger.Warn().
		Str("ref", ref).
		Str("method", ctx.Method()).
		Str("path", ctx.Path()).
		Str("error", chain).
		Msg(message)

	if !signedIn(ctx) {
		return view.NewNotice(message)
	}

	return view.Failure{
		Message: message,
		Details: failure.Details(ref, ctx.Method()+" "+ctx.Path(), chain),
	}
}

// SetFlash keeps a failure in the session for the next page to show, so a
// redirect carries it without putting it in the address.
//
// Parameters:
//   - ctx: The request whose session keeps the failure.
//   - shown: What the next page shows.
func SetFlash(ctx fiber.Ctx, shown view.Failure) {
	sess := session.FromContext(ctx)
	if sess == nil || shown.Empty() {
		return
	}

	sess.Set(sessionKeyFlashMessage, shown.Message)
	sess.Set(sessionKeyFlashDetails, shown.Details)
}

// HasFlash reports whether the session holds a failure for the next page.
//
// Parameters:
//   - ctx: The request whose session is checked.
//
// Returns:
//   - pending: True when a failure waits to be shown.
func HasFlash(ctx fiber.Ctx) bool {
	return !PendingFlash(ctx).Empty()
}

// PendingFlash reads the failure the session holds for the next page without
// clearing it.
//
// Parameters:
//   - ctx: The request whose session is read.
//
// Returns:
//   - failure: The waiting failure, or an empty one.
func PendingFlash(ctx fiber.Ctx) view.Failure {
	sess := session.FromContext(ctx)
	if sess == nil {
		return view.NewNotice("")
	}

	return view.Failure{
		Message: sessionString(sess, sessionKeyFlashMessage),
		Details: sessionString(sess, sessionKeyFlashDetails),
	}
}

// sessionString reads a string the session holds.
//
// Parameters:
//   - sess: The session.
//   - key: The value's key.
//
// Returns:
//   - value: The string, empty when the session holds none under key.
func sessionString(sess *session.Middleware, key string) string {
	value, ok := sess.Get(key).(string)
	if !ok {
		return ""
	}

	return value
}

// takeFlash moves the session's failure onto the request context, where the
// page's banner slot reads it, and clears it so it shows once.
//
// Parameters:
//   - ctx: The request rendering a page.
func takeFlash(ctx fiber.Ctx) {
	shown := PendingFlash(ctx)
	if shown.Empty() {
		return
	}

	sess := session.FromContext(ctx)
	sess.Delete(sessionKeyFlashMessage)
	sess.Delete(sessionKeyFlashDetails)

	ctx.SetContext(view.ContextWithFailure(ctx.Context(), shown))
}

// signedIn reports whether the request belongs to a signed-in session, the
// only one shown a failure's details.
//
// Parameters:
//   - ctx: The request.
//
// Returns:
//   - signedIn: True when the session carries a Plex user and token.
func signedIn(ctx fiber.Ctx) bool {
	sess := session.FromContext(ctx)

	return identity.UserID(sess) != 0 && identity.Token(sess) != ""
}

// scrub removes Plex tokens from text bound for the page or the log: any
// X-Plex-Token value, and the session's own token wherever it appears.
//
// Parameters:
//   - ctx: The request, whose session token is removed by value.
//   - text: The text to clean.
//
// Returns:
//   - clean: The text with every token replaced.
func scrub(ctx fiber.Ctx, text string) string {
	clean := failure.Scrub(text)

	if token := identity.Token(session.FromContext(ctx)); len(token) >= minTokenLength {
		clean = strings.ReplaceAll(clean, token, failure.Redacted)
	}

	return clean
}
