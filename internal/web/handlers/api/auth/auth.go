// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"

	fiber "github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/binding"
	sharedplex "github.com/PapagoLabs/outtake/internal/web/handlers/shared/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
)

// AuthHandler handles Plex PIN and token authentication.
type AuthHandler struct {
	product  string
	clientID string
	baseURL  string
	db       *database.DB
	bind     *binding.Binding
}

const (
	// SessionKeyPinID stores the pending Plex PIN identifier.
	sessionKeyPinID = "plex_pin_id"

	// SessionKeyPinCode stores the pending Plex PIN code.
	sessionKeyPinCode = "plex_pin_code"

	// SessionKeyUserID stores the authenticated Plex user id.
	sessionKeyUserID = "plex_user_id"

	// SessionKeyAuthURL stores the plex.tv authorization URL.
	sessionKeyAuthURL = "plex_auth_url"

	// ClientIDLength is the random client identifier size in bytes.
	clientIDLength = 16

	// StatusWaiting is shown while a PIN is outstanding.
	statusWaiting = "Waiting for Plex authorization..."

	// StatusAuthed is shown after PIN authorization succeeds.
	statusAuthed = "Authenticated! Redirecting..."

	// PersistTokenMsg is logged when storing the Plex token fails.
	persistTokenMsg = "failed to persist token"

	// MsgPlexTokenRequired is shown when the login form is posted empty.
	msgPlexTokenRequired = "Plex token is required"
)

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(
	product, clientID, baseURL string,
	db *database.DB,
	bind *binding.Binding,
) *AuthHandler {
	// Bundle product identity, store, and Plex binding.
	return &AuthHandler{
		product:  product,
		clientID: clientID,
		baseURL:  baseURL,
		db:       db,
		bind:     bind,
	}
}

// Callback completes PIN authorization.
func (handler *AuthHandler) Callback(ctx fiber.Ctx) error {
	sess := session.FromContext(ctx)
	pinID := respond.SessionInt(sess, sessionKeyPinID)
	if pinID == 0 {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathLogin, "No PIN session"))
	}

	pinCode := respond.SessionString(sess, sessionKeyPinCode)
	plexClient := handler.newClient("")

	token, err := plexClient.PollPIN(ctx.Context(), pinID, pinCode)
	if err != nil {
		log.Error().Err(err).Msg("failed to poll PIN")

		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathLogin, "PIN not yet authorized"))
	}

	clearPIN(sess)

	err = handler.storeToken(ctx, token)
	if err != nil {
		log.Error().Err(err).Msg(persistTokenMsg)
	}

	handler.bindServer(ctx, token)

	return sendAuthComplete(ctx, handler.postAuthPath())
}

// Login starts PIN auth or accepts a manual token.
func (handler *AuthHandler) Login(ctx fiber.Ctx) error {
	token := ctx.FormValue("token")
	if token != "" {
		return wrapAuth(handler.finishAuth(ctx, token), "finish auth")
	}

	sess := session.FromContext(ctx)
	if respond.SessionString(sess, middleware.SessionKeyToken) != "" {
		return respond.RedirectTo(ctx, respond.PathRoot)
	}

	if respond.IsFormRequest(ctx) {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathLogin, msgPlexTokenRequired))
	}

	authURL, err := handler.startPIN(ctx, sess)
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadGateway, "pin_failed", err.Error())
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, fiber.Map{"authUrl": authURL})
}

// Logout clears the session and persisted Plex credentials.
func (handler *AuthHandler) Logout(ctx fiber.Ctx) error {
	sess := session.FromContext(ctx)
	if sess != nil {
		err := sess.Reset()
		if err != nil {
			return respond.WriteError(ctx, fiber.StatusInternalServerError, "logout_failed", err.Error())
		}
	}

	if handler.bind != nil {
		handler.bind.Clear()
	}

	if handler.db != nil {
		err := handler.db.ClearAuth(ctx.Context())
		if err != nil {
			log.Warn().Err(err).Msg("failed to clear stored plex credentials")

			return respond.WriteError(ctx, fiber.StatusInternalServerError, "logout_failed", err.Error())
		}
	}

	return respond.RedirectTo(ctx, respond.PathLogin)
}

// Status polls PIN authorization for HTMX.
func (handler *AuthHandler) Status(ctx fiber.Ctx) error {
	sess := session.FromContext(ctx)
	if respond.SessionString(sess, middleware.SessionKeyToken) != "" {
		ctx.Set("HX-Redirect", respond.PathRoot)

		return respond.SendText(ctx, statusAuthed)
	}

	pinID := respond.SessionInt(sess, sessionKeyPinID)
	if pinID == 0 {
		return respond.SendText(ctx, statusWaiting)
	}

	pinCode := respond.SessionString(sess, sessionKeyPinCode)
	plexClient := handler.newClient("")

	pinToken, err := plexClient.PollPIN(ctx.Context(), pinID, pinCode)
	if err != nil {
		return respond.SendText(ctx, statusWaiting)
	}

	clearPIN(sess)

	err = handler.storeToken(ctx, pinToken)
	if err != nil {
		log.Error().Err(err).Msg(persistTokenMsg)
	}

	handler.bindServer(ctx, pinToken)
	ctx.Set("HX-Redirect", handler.postAuthPath())

	return respond.SendText(ctx, statusAuthed)
}

// GenerateClientID returns a random Plex client identifier.
func GenerateClientID() string {
	var buf [clientIDLength]byte

	_, err := rand.Read(buf[:])
	if err != nil {
		return ""
	}

	return hex.EncodeToString(buf[:])
}

// bindServer selects a PMS when exactly one server is discovered.
func (handler *AuthHandler) bindServer(ctx fiber.Ctx, token string) {
	if _, ok := handler.bind.Get(); ok {
		return
	}

	plexClient := handler.newClient(token)

	servers, err := plexClient.DiscoverServers(ctx.Context())
	if err != nil {
		log.Warn().Err(err).Msg("failed to discover plex servers")

		return
	}

	unique := plex.PreferUniqueServers(servers)
	if len(unique) != 1 {
		return
	}

	handler.bind.Set(unique[0])

	err = handler.db.SaveSelectedServer(ctx.Context(), unique[0])
	if err != nil {
		log.Warn().Err(err).Msg("failed to persist selected server")
	}
}

// finishAuth persists the token and redirects after login.
func (handler *AuthHandler) finishAuth(ctx fiber.Ctx, token string) error {
	err := handler.storeToken(ctx, token)
	if err != nil {
		log.Error().Err(err).Msg(persistTokenMsg)
	}

	handler.bindServer(ctx, token)

	return respond.RedirectTo(ctx, handler.postAuthPath())
}

// newClient builds a Plex client for this handler.
func (handler *AuthHandler) newClient(token string) *plex.Client {
	return sharedplex.NewBoundClient(handler.product, handler.clientID, token)
}

// postAuthPath returns the next page after authentication.
func (handler *AuthHandler) postAuthPath() string {
	if _, ok := handler.bind.Get(); ok {
		return respond.PathRoot
	}

	return respond.PathServers
}

// startPIN creates a Plex PIN and returns the Auth App URL.
func (handler *AuthHandler) startPIN(ctx fiber.Ctx, sess *session.Middleware) (string, error) {
	plexClient := handler.newClient("")

	pin, err := plexClient.GeneratePIN(ctx.Context())
	if err != nil {
		return "", fmt.Errorf("generate pin: %w", err)
	}

	sess.Set(sessionKeyPinID, pin.ID)
	sess.Set(sessionKeyPinCode, pin.Code)

	authURL := plexClient.GetAuthURL(
		pin.Code,
		handler.clientID,
		handler.baseURL+"/api/auth/callback",
	)
	sess.Set(sessionKeyAuthURL, authURL)

	return authURL, nil
}

// storeToken writes the access token to the session and database.
func (handler *AuthHandler) storeToken(ctx fiber.Ctx, token string) error {
	sess := session.FromContext(ctx)
	sess.Set(middleware.SessionKeyToken, token)

	plexClient := handler.newClient(token)

	valid, user, err := plexClient.ValidateToken(ctx.Context())
	if err == nil && valid {
		sess.Set(sessionKeyUserID, user.ID)
	}

	err = handler.db.SaveToken(ctx.Context(), handler.clientID, token)
	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	return nil
}

// clearPIN removes pending PIN values from the session.
func clearPIN(sess *session.Middleware) {
	sess.Delete(sessionKeyPinID)
	sess.Delete(sessionKeyPinCode)
	sess.Delete(sessionKeyAuthURL)
}

// sendAuthComplete finishes popup or full-page login after Plex authorizes.
func sendAuthComplete(ctx fiber.Ctx, next string) error {
	ctx.Set(respond.HeaderContentType, respond.ContentTypeHTML)

	page := `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>Signed in</title></head><body>
<script>
(function () {
  var next = ` + strconv.Quote(
		next,
	) + `;
  if (window.opener) {
    window.opener.location = next;
    window.close();
    return;
  }
  window.location = next;
})();
</script>
<p>Signed in. You can close this window.</p>
</body></html>`

	err := ctx.SendString(page)
	if err != nil {
		return fmt.Errorf("send auth complete: %w", err)
	}

	return nil
}

// wrapAuth wraps a handler error with an operation name.
func wrapAuth(err error, op string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
