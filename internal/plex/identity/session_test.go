// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"
)

// failingStorage is session storage that reads nothing and fails every write.
type failingStorage struct{}

// errStorageUnavailable reports session storage that cannot be reached.
var errStorageUnavailable = errors.New("session storage unavailable")

// Close closes the storage.
//
// Returns:
//   - err: Always the storage failure.
func (failingStorage) Close() error {
	return errStorageUnavailable
}

// Delete removes the value for the given key.
//
// Parameters:
//   - key: Session key to remove.
//
// Returns:
//   - err: Always the storage failure.
func (failingStorage) Delete(_ string) error {
	return errStorageUnavailable
}

// DeleteWithContext removes the value for the given key.
//
// Parameters:
//   - ctx: Cancellation and deadline for the operation.
//   - key: Session key to remove.
//
// Returns:
//   - err: Always the storage failure.
func (failingStorage) DeleteWithContext(_ context.Context, _ string) error {
	return errStorageUnavailable
}

// Get returns the value for the given key.
//
// Parameters:
//   - key: Session key to read.
//
// Returns:
//   - value: Always nil, so the caller sees a fresh session.
//   - err: Always nil.
func (failingStorage) Get(_ string) ([]byte, error) {
	return nil, nil
}

// GetWithContext returns the value for the given key.
//
// Parameters:
//   - ctx: Cancellation and deadline for the operation.
//   - key: Session key to read.
//
// Returns:
//   - value: Always nil, so the caller sees a fresh session.
//   - err: Always nil.
func (failingStorage) GetWithContext(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

// Reset removes every stored session.
//
// Returns:
//   - err: Always the storage failure.
func (failingStorage) Reset() error {
	return errStorageUnavailable
}

// ResetWithContext removes every stored session.
//
// Parameters:
//   - ctx: Cancellation and deadline for the operation.
//
// Returns:
//   - err: Always the storage failure.
func (failingStorage) ResetWithContext(_ context.Context) error {
	return errStorageUnavailable
}

// Set stores the value for the given key.
//
// Parameters:
//   - key: Session key to write.
//   - value: Session value to write.
//   - exp: Expiration for the stored value.
//
// Returns:
//   - err: Always the storage failure.
func (failingStorage) Set(_ string, _ []byte, _ time.Duration) error {
	return errStorageUnavailable
}

// SetWithContext stores the value for the given key.
//
// Parameters:
//   - ctx: Cancellation and deadline for the operation.
//   - key: Session key to write.
//   - value: Session value to write.
//   - exp: Expiration for the stored value.
//
// Returns:
//   - err: Always the storage failure.
func (failingStorage) SetWithContext(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return errStorageUnavailable
}

// withTestSession runs body against a live Fiber session.
//
// Parameters:
//   - t: The test the session belongs to.
//   - config: Session configuration, where an empty config uses the default
//     in-memory storage.
//   - body: Function invoked with the live session middleware.
func withTestSession(t *testing.T, config session.Config, body func(sess *session.Middleware)) {
	t.Helper()

	app := fiber.New()

	middleware, _ := session.NewWithStore(config)

	app.Use(middleware)
	app.Get("/session", func(c fiber.Ctx) error {
		body(session.FromContext(c))

		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/session", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestClearStoredPINRemovesThePendingValues(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetStoredPIN(sess, 4321, "PIN-CODE")
		SetToken(sess, "session-token")

		ClearStoredPIN(sess)

		pinID, pinCode := StoredPIN(sess)
		assert.Zero(t, pinID, "the pending PIN identifier is dropped")
		assert.Empty(t, pinCode, "the pending PIN code is dropped")
		assert.Equal(t, "session-token", Token(sess), "the access token survives")
	})
}

func TestClearStoredPINWithoutASession(t *testing.T) {
	t.Parallel()

	ClearStoredPIN(nil)
}

func TestResetClearsEveryValue(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetToken(sess, "session-token")
		SetUserID(sess, 77)
		SetStoredPIN(sess, 4321, "PIN-CODE")

		require.NoError(t, Reset(sess))

		assert.Empty(t, Token(sess))
		assert.Zero(t, storedInt(sess, SessionKeyUserID))
		assert.Zero(t, storedInt(sess, SessionKeyPinID))
		assert.Empty(t, storedString(sess, SessionKeyPinCode))
	})
}

func TestResetWithoutASession(t *testing.T) {
	t.Parallel()

	require.NoError(t, Reset(nil))
}

func TestResetReportsAFailedStorage(t *testing.T) {
	t.Parallel()

	config := session.Config{
		Storage:      failingStorage{},
		ErrorHandler: func(fiber.Ctx, error) {},
	}

	withTestSession(t, config, func(sess *session.Middleware) {
		SetToken(sess, "session-token")

		err := Reset(sess)
		require.ErrorIs(t, err, errStorageUnavailable)
		assert.ErrorContains(t, err, "reset session")
	})
}

func TestSetStoredPINRoundTrips(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetStoredPIN(sess, 4321, "PIN-CODE")

		pinID, pinCode := StoredPIN(sess)
		assert.Equal(t, 4321, pinID)
		assert.Equal(t, "PIN-CODE", pinCode)
	})
}

func TestSetStoredPINOverwritesThePendingValues(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetStoredPIN(sess, 4321, "PIN-CODE")
		SetStoredPIN(sess, 9999, "OTHER")

		pinID, pinCode := StoredPIN(sess)
		assert.Equal(t, 9999, pinID)
		assert.Equal(t, "OTHER", pinCode)
	})
}

func TestSetStoredPINWithoutASession(t *testing.T) {
	t.Parallel()

	SetStoredPIN(nil, 4321, "PIN-CODE")
}

func TestSetTokenRoundTrips(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetToken(sess, "session-token")

		assert.Equal(t, "session-token", Token(sess))
	})
}

func TestSetTokenOverwritesTheStoredToken(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetToken(sess, "session-token")
		SetToken(sess, "replacement-token")

		assert.Equal(t, "replacement-token", Token(sess))
	})
}

func TestSetTokenSkipsAnEmptyToken(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetToken(sess, "session-token")
		SetToken(sess, "")

		assert.Equal(t, "session-token", Token(sess), "an empty token leaves the stored one alone")
	})
}

func TestSetTokenWithoutASession(t *testing.T) {
	t.Parallel()

	SetToken(nil, "session-token")
}

func TestSetUserIDRoundTrips(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetUserID(sess, 77)

		assert.Equal(t, 77, storedInt(sess, SessionKeyUserID))
	})
}

func TestSetUserIDOverwritesTheStoredID(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetUserID(sess, 77)
		SetUserID(sess, 1234)

		assert.Equal(t, 1234, storedInt(sess, SessionKeyUserID))
	})
}

func TestSetUserIDSkipsAZeroID(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		SetUserID(sess, 77)
		SetUserID(sess, 0)

		assert.Equal(t, 77, storedInt(sess, SessionKeyUserID))
	})
}

func TestSetUserIDWithoutASession(t *testing.T) {
	t.Parallel()

	SetUserID(nil, 77)
}

func TestStoredPINWithoutASession(t *testing.T) {
	t.Parallel()

	pinID, pinCode := StoredPIN(nil)
	assert.Zero(t, pinID)
	assert.Empty(t, pinCode)
}

func TestStoredPINWithoutAPendingValue(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		pinID, pinCode := StoredPIN(sess)
		assert.Zero(t, pinID)
		assert.Empty(t, pinCode)
	})
}

func TestStoredPINIgnoresMismatchedTypes(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		sess.Set(SessionKeyPinID, "not-an-int")
		sess.Set(SessionKeyPinCode, 1234)

		pinID, pinCode := StoredPIN(sess)
		assert.Zero(t, pinID, "a string where an int is expected reads as unset")
		assert.Empty(t, pinCode, "an int where a string is expected reads as unset")
	})
}

func TestTokenWithoutASession(t *testing.T) {
	t.Parallel()

	assert.Empty(t, Token(nil))
}

func TestTokenIgnoresMismatchedTypes(t *testing.T) {
	t.Parallel()

	withTestSession(t, session.Config{}, func(sess *session.Middleware) {
		sess.Set(SessionKeyToken, 1234)

		assert.Empty(t, Token(sess), "an int where a string is expected reads as unset")
	})
}

func TestStoredIntReadsEveryForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		expected int
	}{
		{name: "an int is read back", value: 77, expected: 77},
		{name: "a string reads as unset", value: "session-token", expected: 0},
		{name: "a float reads as unset", value: 1.5, expected: 0},
		{name: "nil reads as unset", value: nil, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			withTestSession(t, session.Config{}, func(sess *session.Middleware) {
				sess.Set(SessionKeyUserID, tt.value)

				assert.Equal(t, tt.expected, storedInt(sess, SessionKeyUserID))
			})
		})
	}
}

func TestStoredIntWithoutASession(t *testing.T) {
	t.Parallel()

	assert.Zero(t, storedInt(nil, SessionKeyUserID))
}

func TestStoredStringReadsEveryForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{name: "a string is read back", value: "session-token", expected: "session-token"},
		{name: "an int reads as unset", value: 77, expected: ""},
		{name: "a bool reads as unset", value: true, expected: ""},
		{name: "nil reads as unset", value: nil, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			withTestSession(t, session.Config{}, func(sess *session.Middleware) {
				sess.Set(SessionKeyToken, tt.value)

				assert.Equal(t, tt.expected, storedString(sess, SessionKeyToken))
			})
		})
	}
}

func TestStoredStringWithoutASession(t *testing.T) {
	t.Parallel()

	assert.Empty(t, storedString(nil, SessionKeyToken))
}
