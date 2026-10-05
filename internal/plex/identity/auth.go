// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// TokenStore persists and clears the Plex credentials of this installation.
type TokenStore interface {
	// SaveToken records the access token issued for a client identifier.
	SaveToken(ctx context.Context, clientID, accessToken string) error

	// ClearAuth removes every persisted Plex credential.
	ClearAuth(ctx context.Context) error

	// LatestToken returns the most recently persisted access token.
	LatestToken(ctx context.Context) (string, error)

	// SaveSelectedServer records the Plex server chosen by the user.
	SaveSelectedServer(ctx context.Context, server plex.Server) error
}

// ServerBinding holds the process-wide Plex server selection and the live
// sessions it polls.
type ServerBinding interface {
	// Get returns the selected server.
	Get() (plex.Server, bool)

	// Set selects a server.
	Set(server plex.Server)

	// Clear drops the selected server.
	Clear()

	// Client builds a Plex client for the selected server.
	Client() (*plex.Client, plex.Server, bool)

	// Sessions returns the latest cached playback sessions.
	Sessions() []plex.Session
}

// PendingPIN is a Plex PIN that has been created but not yet authorized.
type PendingPIN struct {
	// ID is the Plex PIN identifier used when polling for authorization.
	ID int

	// Code is the Plex PIN code shown to the user.
	Code string

	// URL is the Plex Auth App URL the browser is sent to.
	URL string
}

// Auth coordinates the Plex PIN login lifecycle.
type Auth struct {
	product  string
	clientID string
	baseURL  string
	store    TokenStore
	selected ServerBinding

	// newClient builds the Plex client behind every Plex call. A nil value
	// means plex.NewClient.
	newClient func(plex.ClientConfig) *plex.Client
}

// ClientIDLength is the random client identifier size in bytes.
const ClientIDLength = 16

// forwardPath is the route Plex redirects to once the user authorizes a PIN.
const forwardPath = "/api/auth/callback"

// ErrInvalidToken reports a Plex token the server does not accept.
var ErrInvalidToken = errors.New("plex token was rejected")

// New creates the Plex authentication service.
//
// Parameters:
//   - product: Plex product name sent with API requests.
//   - clientID: Plex client identifier.
//   - baseURL: Public base URL of this server, used to build the Plex forward URL.
//   - store: Token persistence, which may be nil to disable persistence.
//   - selected: Plex server binding, which may be nil to disable server selection.
//
// Returns:
//   - auth: A ready-to-use Plex authentication service.
func New(
	product, clientID, baseURL string,
	store TokenStore,
	selected ServerBinding,
) *Auth {
	return &Auth{
		product:  product,
		clientID: clientID,
		baseURL:  baseURL,
		store:    store,
		selected: selected,
	}
}

// Accept takes a Plex access token as authenticated.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token to accept.
//
// Returns:
//   - userID: Plex user id, or zero when the token did not validate.
//   - err: Wrapped error when the token could not be persisted.
func (auth *Auth) Accept(ctx context.Context, accessToken string) (int, error) {
	userID := auth.validate(ctx, accessToken)

	err := auth.saveToken(ctx, accessToken)

	auth.bindServer(ctx, accessToken)

	if err != nil {
		return userID, fmt.Errorf("accept token: %w", err)
	}

	return userID, nil
}

// BeginPIN creates a Plex PIN and returns it with the Plex Auth App URL the
// browser must be sent to in order to authorize it.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - pin: The created PIN and its authorization URL.
//   - err: Wrapped error when Plex could not create a PIN.
func (auth *Auth) BeginPIN(ctx context.Context) (PendingPIN, error) {
	client := auth.clientFor("")

	pin, err := client.GeneratePIN(ctx)
	if err != nil {
		return PendingPIN{}, fmt.Errorf("generate pin: %w", err)
	}

	forwardURL := auth.baseURL + forwardPath

	return PendingPIN{
		ID:   pin.ID,
		Code: pin.Code,
		URL:  client.GetAuthURL(pin.Code, auth.clientID, forwardURL),
	}, nil
}

// Bound reports whether a Plex server is already selected.
//
// Returns:
//   - bound: True when a server has been selected.
func (auth *Auth) Bound() bool {
	if auth.selected == nil {
		return false
	}

	_, ok := auth.selected.Get()

	return ok
}

// Client builds a Plex client for the selected server.
//
// Returns:
//   - client: A Plex client scoped to the selected server.
//   - server: The selected server.
//   - ok: False when no server has been selected.
func (auth *Auth) Client() (*plex.Client, plex.Server, bool) {
	if auth.selected == nil {
		return nil, plex.EmptyServer(), false
	}

	return auth.selected.Client()
}

// Discover lists the Plex servers an access token can reach, so a client can be
// pointed at one.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token to discover servers with.
//
// Returns:
//   - servers: Every connection Plex reported, in the order it reported them.
//   - err: Wrapped error when Plex could not be asked.
func (auth *Auth) Discover(ctx context.Context, accessToken string) ([]plex.Server, error) {
	servers, err := auth.clientFor(accessToken).DiscoverServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover servers: %w", err)
	}

	return servers, nil
}

// Login validates a manually entered Plex access token and records it, so that a
// token the server rejects never reaches the session or the store.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token the user entered.
//
// Returns:
//   - userID: Plex user id behind the token.
//   - err: ErrInvalidToken when Plex rejects the token, otherwise the wrapped
//     failure to persist it.
func (auth *Auth) Login(ctx context.Context, accessToken string) (int, error) {
	userID := auth.validate(ctx, accessToken)
	if userID == 0 {
		return 0, ErrInvalidToken
	}

	err := auth.saveToken(ctx, accessToken)

	auth.bindServer(ctx, accessToken)

	if err != nil {
		return userID, fmt.Errorf("login: %w", err)
	}

	return userID, nil
}

// Logout clears the selected Plex server and the persisted Plex credentials.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Wrapped error when the persisted credentials could not be cleared.
func (auth *Auth) Logout(ctx context.Context) error {
	if auth.selected != nil {
		auth.selected.Clear()
	}

	if auth.store == nil {
		return nil
	}

	err := auth.store.ClearAuth(ctx)
	if err != nil {
		return fmt.Errorf("clear auth: %w", err)
	}

	return nil
}

// PollPIN asks Plex whether the pending PIN has been authorized.
//
// Parameters:
//   - ctx: Request context.
//   - pinID: Plex PIN identifier.
//   - pinCode: Plex PIN code.
//
// Returns:
//   - accessToken: Plex access token once the PIN has been authorized.
//   - err: Wrapped error, which is also the answer while the PIN is still unauthorized.
func (auth *Auth) PollPIN(ctx context.Context, pinID int, pinCode string) (string, error) {
	accessToken, err := auth.clientFor("").PollPIN(ctx, pinID, pinCode)
	if err != nil {
		return "", fmt.Errorf("poll pin: %w", err)
	}

	return accessToken, nil
}

// Select binds a Plex server the user choses and persists the choice.
//
// Parameters:
//   - ctx: Request context.
//   - server: Server to bind.
//
// Returns:
//   - err: Wrapped error when the choice could not be persisted.
func (auth *Auth) Select(ctx context.Context, server plex.Server) error {
	if auth.selected != nil {
		auth.selected.Set(server)
	}

	if auth.store == nil {
		return nil
	}

	err := auth.store.SaveSelectedServer(ctx, server)
	if err != nil {
		return fmt.Errorf("save selected server: %w", err)
	}

	return nil
}

// Selected returns the Plex server this installation is bound to.
//
// Returns:
//   - server: The selected server.
//   - ok: False when no server has been selected.
func (auth *Auth) Selected() (plex.Server, bool) {
	if auth.selected == nil {
		return plex.EmptyServer(), false
	}

	return auth.selected.Get()
}

// Sessions returns the live Plex playback sessions of the selected server.
//
// Returns:
//   - sessions: The cached sessions, or nil when no server is selected.
func (auth *Auth) Sessions() []plex.Session {
	if auth.selected == nil {
		return nil
	}

	return auth.selected.Sessions()
}

// bindServer selects the single discovered Plex server and persists the choice.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token to discover servers with.
func (auth *Auth) bindServer(ctx context.Context, accessToken string) {
	if auth.selected == nil || auth.store == nil {
		return
	}

	if _, ok := auth.selected.Get(); ok {
		return
	}

	servers, err := auth.clientFor(accessToken).DiscoverServers(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to discover plex servers")

		return
	}

	unique := plex.PreferUniqueServers(servers)
	if len(unique) != 1 {
		return
	}

	auth.selected.Set(unique[0])

	saveErr := auth.store.SaveSelectedServer(ctx, unique[0])
	if saveErr != nil {
		log.Warn().Err(saveErr).Msg("failed to persist selected server")
	}
}

// clientFor builds a Plex client that presents the given access token.
//
// Parameters:
//   - accessToken: Plex access token, or empty for an unauthenticated client.
//
// Returns:
//   - client: A Plex client scoped to this service identity.
func (auth *Auth) clientFor(accessToken string) *plex.Client {
	build := auth.newClient
	if build == nil {
		build = plex.NewClient
	}

	return build(plex.ClientConfig{
		Product:  auth.product,
		ClientID: auth.clientID,
		Token:    accessToken,
		Timeout:  0,
		BaseURL:  "",
	})
}

// saveToken writes an access token to the token store.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token to persist.
//
// Returns:
//   - err: Wrapped error when the token could not be persisted.
func (auth *Auth) saveToken(ctx context.Context, accessToken string) error {
	if auth.store == nil {
		return nil
	}

	err := auth.store.SaveToken(ctx, auth.clientID, accessToken)
	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	return nil
}

// validate resolves the Plex user behind an access token.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token to validate.
//
// Returns:
//   - userID: Plex user id, or zero when the token did not validate.
func (auth *Auth) validate(ctx context.Context, accessToken string) int {
	valid, user, err := auth.clientFor(accessToken).ValidateToken(ctx)
	if err != nil || !valid || user == nil {
		return 0
	}

	return user.ID
}

// GenerateClientID returns a random Plex client identifier.
//
// Returns:
//   - id: Hex-encoded random identifier, or empty when randomness is unavailable.
func GenerateClientID() string {
	var buf [ClientIDLength]byte

	_, err := rand.Read(buf[:])
	if err != nil {
		return ""
	}

	return hex.EncodeToString(buf[:])
}

// Restore returns the access token persisted by an earlier login.
//
// Parameters:
//   - ctx: Request context.
//   - store: Token store to read, which may be nil.
//
// Returns:
//   - accessToken: Plex access token, or empty when none is stored or the
//     read failed.
func Restore(ctx context.Context, store TokenStore) string {
	if store == nil {
		return ""
	}

	accessToken, err := store.LatestToken(ctx)
	if err != nil {
		return ""
	}

	return accessToken
}
