// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"fmt"

	"github.com/gofiber/fiber/v3/middleware/session"
)

// Session keys used by the Fiber session store.
const (
	// SessionKeyToken stores the authenticated Plex access token.
	SessionKeyToken = "plex_token"

	// SessionKeyPinID stores the pending Plex PIN identifier.
	SessionKeyPinID = "plex_pin_id"

	// SessionKeyPinCode stores the pending Plex PIN code.
	SessionKeyPinCode = "plex_pin_code"

	// SessionKeyUserID stores the authenticated Plex user id.
	SessionKeyUserID = "plex_user_id"
)

// ClearStoredPIN removes the pending Plex PIN values from the session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
func ClearStoredPIN(sess *session.Middleware) {
	if sess == nil {
		return
	}

	sess.Delete(SessionKeyPinID)
	sess.Delete(SessionKeyPinCode)
}

// Reset clears every value on the session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
//
// Returns:
//   - err: Wrapped error when the session could not be reset.
func Reset(sess *session.Middleware) error {
	if sess == nil {
		return nil
	}

	err := sess.Reset()
	if err != nil {
		return fmt.Errorf("reset session: %w", err)
	}

	return nil
}

// SetStoredPIN stores a newly created Plex PIN on the session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
//   - pinID: Plex PIN identifier.
//   - pinCode: Plex PIN code.
func SetStoredPIN(sess *session.Middleware, pinID int, pinCode string) {
	if sess == nil {
		return
	}

	sess.Set(SessionKeyPinID, pinID)
	sess.Set(SessionKeyPinCode, pinCode)
}

// SetToken stores the Plex access token on the session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
//   - accessToken: Plex access token.
func SetToken(sess *session.Middleware, accessToken string) {
	if sess == nil || accessToken == "" {
		return
	}

	sess.Set(SessionKeyToken, accessToken)
}

// SetUserID stores the authenticated Plex user id on the session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
//   - userID: Plex user id.
func SetUserID(sess *session.Middleware, userID int) {
	if sess == nil || userID == 0 {
		return
	}

	sess.Set(SessionKeyUserID, userID)
}

// StoredPIN returns the Plex PIN awaiting authorization on the session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
//
// Returns:
//   - pinID: Plex PIN identifier, or zero when it is not set.
//   - pinCode: Plex PIN code, or empty when it is not set.
func StoredPIN(sess *session.Middleware) (int, string) {
	return storedInt(sess, SessionKeyPinID), storedString(sess, SessionKeyPinCode)
}

// Token returns the Plex access token stored on the session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
//
// Returns:
//   - token: Plex access token, or empty when the session carries none.
func Token(sess *session.Middleware) string {
	return storedString(sess, SessionKeyToken)
}

// UserID returns the Plex user id stored on the session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
//
// Returns:
//   - userID: Plex user id, or zero when the session carries none.
func UserID(sess *session.Middleware) int {
	return storedInt(sess, SessionKeyUserID)
}

// storedInt reads an int value from the Fiber session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
//   - key: Session key to read.
//
// Returns:
//   - Stored int, or zero when the key is absent or holds another type.
func storedInt(sess *session.Middleware, key string) int {
	if sess == nil {
		return 0
	}

	value, ok := sess.Get(key).(int)
	if !ok {
		return 0
	}

	return value
}

// storedString reads a string value from the Fiber session.
//
// Parameters:
//   - sess: Fiber session, which may be nil.
//   - key: Session key to read.
//
// Returns:
//   - Stored string, or empty when the key is absent or holds another type.
func storedString(sess *session.Middleware, key string) string {
	if sess == nil {
		return ""
	}

	value, ok := sess.Get(key).(string)
	if !ok {
		return ""
	}

	return value
}
