// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// Store persists who this installation belongs to and the Plex server it is
// bound to.
type Store interface {
	// SaveSelectedServer records the Plex server chosen by the owner.
	SaveSelectedServer(ctx context.Context, server plex.Server) error

	// ClearSelectedServer forgets the persisted server selection.
	ClearSelectedServer(ctx context.Context) error

	// UserRole returns the role of the user behind a Plex account, reporting
	// whether the account belongs to a user.
	UserRole(ctx context.Context, plexUserID int) (string, bool, error)

	// HasOwner reports whether a Plex account has claimed this installation.
	HasOwner(ctx context.Context) (bool, error)

	// ClaimOwner records a Plex account as the owner unless one is stored
	// already, reporting whether this call stored it.
	ClaimOwner(ctx context.Context, plexUserID int, username string) (bool, error)

	// TouchLogin records a sign-in.
	TouchLogin(ctx context.Context, plexUserID int, username string) error

	// LegacyToken returns the newest stored Plex token, or empty when none is
	// stored.
	LegacyToken(ctx context.Context) (string, error)

	// ClearLegacyTokens deletes every stored Plex token.
	ClearLegacyTokens(ctx context.Context) error
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

	// TokenRejected reports whether the server refused the stored token.
	TokenRejected() bool
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

// Auth coordinates the Plex PIN login lifecycle and decides which Plex accounts
// may use this installation.
type Auth struct {
	product  string
	clientID string
	baseURL  string
	plexURL  string
	store    Store
	selected ServerBinding
}

// Option adjusts an authentication service while New builds it.
type Option func(*Auth)

// ClientIDLength is the random client identifier size in bytes.
const ClientIDLength = 16

// forwardPath is the route Plex redirects to once the user authorizes a PIN.
const forwardPath = "/api/auth/callback"

// identityTimeout bounds the identity check of a custom server URL.
const identityTimeout = 5 * time.Second

var (
	// ErrInvalidToken reports a Plex token the server does not accept.
	ErrInvalidToken = errors.New("plex token was rejected")

	// ErrNotAllowed reports a Plex account this installation does not belong to.
	ErrNotAllowed = errors.New("plex account may not use this installation")

	// ErrServerNotFound reports a server choice that matches no server the
	// account can reach through plex.tv.
	ErrServerNotFound = errors.New("plex server is not on this account")

	// ErrServerUnreachable reports a server this installation cannot connect to.
	ErrServerUnreachable = errors.New("plex server cannot be reached")

	// ErrInvalidServerURL reports a custom server URL that is not http or https.
	ErrInvalidServerURL = errors.New("invalid plex server url")
)

// New creates the Plex authentication service.
//
// Parameters:
//   - product: Plex product name sent with API requests.
//   - clientID: Plex client identifier.
//   - baseURL: Public base URL of this server, used to build the Plex forward URL.
//   - store: Persistence for the owner and the selected server. A nil store
//     refuses every sign-in.
//   - selected: Plex server binding, which may be nil to disable server selection.
//   - opts: Adjustments applied after the defaults.
//
// Returns:
//   - auth: A ready-to-use Plex authentication service.
func New(
	product, clientID, baseURL string,
	store Store,
	selected ServerBinding,
	opts ...Option,
) *Auth {
	auth := &Auth{
		product:  product,
		clientID: clientID,
		baseURL:  baseURL,
		plexURL:  "",
		store:    store,
		selected: selected,
	}

	for _, opt := range opts {
		opt(auth)
	}

	return auth
}

// WithPlexURL sends every Plex request to origin instead of plex.tv.
//
// Parameters:
//   - origin: Scheme and host Plex requests are aimed at.
//
// Returns:
//   - opt: The option New applies.
func WithPlexURL(origin string) Option {
	return func(auth *Auth) {
		auth.plexURL = origin
	}
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

// ChooseCustomURL resolves a server URL the owner typed. The URL is asked for
// its identity without a token, and the server's token is used only when that
// identity matches a server the account can see, so no token is sent to a
// host that is not one of the account's servers.
//
// The URL need not be a connection plex.tv lists, because it exists for
// addresses Plex does not know, such as a container name or a reverse proxy.
// A machine identifier is not secret, so a host that relays the server's
// identity passes the check. Only the signed-in owner can submit a URL, which
// limits that to a host the owner typed.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token servers are discovered with.
//   - rawURL: The URL the owner typed.
//
// Returns:
//   - server: The connection the URL names, with its server's token.
//   - err: ErrInvalidServerURL, ErrServerUnreachable, ErrServerNotFound, or the
//     wrapped discovery failure.
func (auth *Auth) ChooseCustomURL(
	ctx context.Context,
	accessToken, rawURL string,
) (plex.Server, error) {
	custom, ok := plex.ParseServerURL(rawURL)
	if !ok {
		return plex.EmptyServer(), ErrInvalidServerURL
	}

	identityCtx, cancel := context.WithTimeout(ctx, identityTimeout)
	defer cancel()

	identity, err := auth.clientFor(accessToken).GetServerIdentity(identityCtx, custom)
	if err != nil || identity.MachineIdentifier == "" {
		return plex.EmptyServer(), ErrServerUnreachable
	}

	servers, err := auth.Discover(ctx, accessToken)
	if err != nil {
		return plex.EmptyServer(), err
	}

	for _, server := range servers {
		if server.MachineID != identity.MachineIdentifier {
			continue
		}

		custom.Name = server.Name
		custom.Token = server.Token
		custom.MachineID = server.MachineID

		return custom, nil
	}

	return plex.EmptyServer(), ErrServerNotFound
}

// ChooseServer resolves a discovered connection the owner picked by its
// selection key. The servers are discovered again, so the token comes from
// Plex rather than from the request.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token servers are discovered with.
//   - key: The connection's plex.SelectionKey.
//
// Returns:
//   - server: The chosen connection.
//   - err: ErrServerNotFound, ErrServerUnreachable, or the wrapped discovery
//     failure.
func (auth *Auth) ChooseServer(ctx context.Context, accessToken, key string) (plex.Server, error) {
	servers, err := auth.Discover(ctx, accessToken)
	if err != nil {
		return plex.EmptyServer(), err
	}

	client := auth.clientFor(accessToken)

	for _, server := range servers {
		if plex.SelectionKey(server) != key {
			continue
		}

		if client.Ping(ctx, server) != nil {
			return plex.EmptyServer(), ErrServerUnreachable
		}

		return server, nil
	}

	return plex.EmptyServer(), ErrServerNotFound
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

// ForgetServer drops the persisted Plex server record and then the live
// selection, so the owner can pick another. When the record cannot be cleared,
// the live selection is kept.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Wrapped error when the persisted record could not be cleared.
func (auth *Auth) ForgetServer(ctx context.Context) error {
	if auth.store != nil {
		err := auth.store.ClearSelectedServer(ctx)
		if err != nil {
			return fmt.Errorf("clear selected server: %w", err)
		}
	}

	if auth.selected != nil {
		auth.selected.Clear()
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

// SignIn authenticates a Plex access token as a user of this installation.
//
// The first Plex account to sign in claims the installation as its owner. When
// a Plex token is stored, only the account behind that token may claim it.
// Every other account is refused. A token that is invalid or
// refused is never used to bind a server.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token to sign in with.
//
// Returns:
//   - user: The user the token signs in as.
//   - err: ErrInvalidToken when Plex rejects the token, ErrNotAllowed when the
//     account may not use this installation, otherwise the wrapped store failure.
func (auth *Auth) SignIn(ctx context.Context, accessToken string) (User, error) {
	candidate, ok := auth.validate(ctx, accessToken)
	if !ok {
		return User{}, ErrInvalidToken
	}

	user, err := auth.authorize(ctx, candidate)
	if err != nil {
		return User{}, fmt.Errorf("authorize: %w", err)
	}

	auth.bindServer(ctx, accessToken)

	return user, nil
}

// TokenRejected reports whether the bound server refused its stored token on
// the last session poll. Signing in again refreshes the token.
//
// Returns:
//   - rejected: True while the bound server refuses the token.
func (auth *Auth) TokenRejected() bool {
	if auth.selected == nil {
		return false
	}

	return auth.selected.TokenRejected()
}

// authorize decides whether a validated Plex account may use this installation.
//
// Parameters:
//   - ctx: Request context.
//   - candidate: The account Plex vouched for.
//
// Returns:
//   - user: The stored user the account signs in as.
//   - err: ErrNotAllowed for a refused account, otherwise the wrapped store failure.
func (auth *Auth) authorize(ctx context.Context, candidate User) (User, error) {
	if auth.store == nil {
		return User{}, ErrNotAllowed
	}

	role, found, err := auth.store.UserRole(ctx, candidate.PlexID)
	if err != nil {
		return User{}, fmt.Errorf("look up user: %w", err)
	}

	if !found {
		owner, claimErr := auth.claimOwner(ctx, candidate)
		if claimErr != nil {
			return User{}, fmt.Errorf("claim installation: %w", claimErr)
		}

		return owner, nil
	}

	touchErr := auth.store.TouchLogin(ctx, candidate.PlexID, candidate.Username)
	if touchErr != nil {
		log.Warn().Err(touchErr).Msg("failed to record plex sign-in")
	}

	candidate.Role = Role(role)

	return candidate, nil
}

// bindServer selects the single discovered Plex server, through its most
// preferred connection that answers, and persists the choice. When a server is
// bound already, its token is refreshed instead.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token to discover servers with.
func (auth *Auth) bindServer(ctx context.Context, accessToken string) {
	if auth.selected == nil || auth.store == nil {
		return
	}

	client := auth.clientFor(accessToken)

	if bound, ok := auth.selected.Get(); ok {
		auth.refreshBoundToken(ctx, client, bound)

		return
	}

	servers, err := client.DiscoverServers(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to discover plex servers")

		return
	}

	unique := plex.PreferUniqueServers(servers)
	if len(unique) != 1 {
		return
	}

	reachable, ok := client.FirstReachable(ctx, plex.ConnectionsOf(servers, unique[0]))
	if !ok {
		log.Warn().
			Str("server", unique[0].Name).
			Msg("no connection to the only plex server answered")

		return
	}

	auth.selected.Set(reachable)

	saveErr := auth.store.SaveSelectedServer(ctx, reachable)
	if saveErr != nil {
		log.Warn().Err(saveErr).Msg("failed to persist selected server")
	}
}

// claimOwner makes a validated Plex account the owner of an installation that
// has none.
//
// Parameters:
//   - ctx: Request context.
//   - candidate: The account Plex vouched for.
//
// Returns:
//   - user: The owner.
//   - err: ErrNotAllowed when the installation is owned or held for another
//     account, otherwise the wrapped store failure.
func (auth *Auth) claimOwner(ctx context.Context, candidate User) (User, error) {
	owned, err := auth.store.HasOwner(ctx)
	if err != nil {
		return User{}, fmt.Errorf("check owner: %w", err)
	}

	if owned || !auth.legacyOwnerAllows(ctx, candidate.PlexID) {
		return User{}, ErrNotAllowed
	}

	claimed, err := auth.store.ClaimOwner(ctx, candidate.PlexID, candidate.Username)
	if err != nil {
		return User{}, fmt.Errorf("claim owner: %w", err)
	}

	if !claimed {
		winner, winnerErr := auth.claimWinner(ctx, candidate)
		if winnerErr != nil {
			return User{}, fmt.Errorf("resolve lost claim: %w", winnerErr)
		}

		return winner, nil
	}

	return auth.settleClaim(ctx, candidate), nil
}

// claimWinner resolves a claim another sign-in won, which the same account may
// have made from a second browser.
//
// Parameters:
//   - ctx: Request context.
//   - candidate: The account that lost the claim.
//
// Returns:
//   - user: The account with its stored role when it won the claim itself.
//   - err: ErrNotAllowed when another account won, otherwise the wrapped store
//     failure.
func (auth *Auth) claimWinner(ctx context.Context, candidate User) (User, error) {
	role, found, err := auth.store.UserRole(ctx, candidate.PlexID)
	if err != nil {
		return User{}, fmt.Errorf("look up user: %w", err)
	}

	if !found {
		return User{}, ErrNotAllowed
	}

	candidate.Role = Role(role)

	return candidate, nil
}

// clientFor builds a Plex client that presents the given access token.
//
// Parameters:
//   - accessToken: Plex access token, or empty for an unauthenticated client.
//
// Returns:
//   - client: A Plex client scoped to this service identity.
func (auth *Auth) clientFor(accessToken string) *plex.Client {
	return plex.NewClient(plex.ClientConfig{
		Product:  auth.product,
		ClientID: auth.clientID,
		Token:    accessToken,
		Timeout:  0,
		BaseURL:  auth.plexURL,
	})
}

// legacyOwnerAllows reports whether an account may claim the installation.
// When a Plex token is stored, only the account behind it may. A stored token
// Plex rejects as unauthorized names no account, so it does not restrict the
// claim. Any other failure to resolve the stored token refuses the claim.
//
// Parameters:
//   - ctx: Request context.
//   - plexUserID: The account claiming ownership.
//
// Returns:
//   - allowed: True when nothing reserves the installation for another account.
func (auth *Auth) legacyOwnerAllows(ctx context.Context, plexUserID int) bool {
	legacyToken, err := auth.store.LegacyToken(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to read the legacy plex token")

		return false
	}

	if legacyToken == "" {
		return true
	}

	valid, account, err := auth.clientFor(legacyToken).ValidateToken(ctx)
	if errors.Is(err, plex.ErrUnauthorized) {
		log.Warn().
			Msg("the stored plex token is unauthorized, so the next sign-in claims this installation")

		return true
	}

	if err != nil || !valid || account == nil || account.ID == 0 {
		log.Warn().Err(err).Msg("failed to resolve the account behind the stored plex token")

		return false
	}

	return account.ID == plexUserID
}

// refreshBoundToken replaces the bound server's stored token with the one the
// signed-in account's discovery reports for the same server, matched by
// machine identifier. A server token can change after it was bound, and Plex
// then refuses every request made with the stored one. The connection itself,
// which may be a custom URL, is kept.
//
// Parameters:
//   - ctx: Request context.
//   - client: Plex client carrying the signed-in account's token.
//   - bound: The server bound now.
func (auth *Auth) refreshBoundToken(ctx context.Context, client *plex.Client, bound plex.Server) {
	machineID := boundMachineID(ctx, client, bound)
	if machineID == "" {
		log.Warn().Str("server", bound.Name).Msg("could not identify the bound plex server")

		return
	}

	servers, err := client.DiscoverServers(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to discover plex servers")

		return
	}

	token, found := discoveredToken(servers, machineID)
	if !found {
		log.Warn().
			Str("server", bound.Name).
			Msg("the bound plex server is not among the signed-in account's servers")

		return
	}

	if token == bound.Token && bound.MachineID == machineID {
		return
	}

	refreshed := bound

	refreshed.Token = token
	refreshed.MachineID = machineID

	auth.selected.Set(refreshed)

	saveErr := auth.store.SaveSelectedServer(ctx, refreshed)
	if saveErr != nil {
		log.Warn().Err(saveErr).Msg("failed to persist refreshed plex server token")

		return
	}

	log.Info().Str("server", bound.Name).Msg("refreshed the bound plex server's token")
}

// settleClaim completes a claim this sign-in won and deletes the stored Plex
// tokens.
//
// Parameters:
//   - ctx: Request context.
//   - candidate: The account that claimed the installation.
//
// Returns:
//   - owner: The account as the owner.
func (auth *Auth) settleClaim(ctx context.Context, candidate User) User {
	err := auth.store.ClearLegacyTokens(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to clear legacy plex tokens")
	}

	log.Info().
		Int("plex_user_id", candidate.PlexID).
		Str("username", candidate.Username).
		Msg("plex account claimed this installation")

	candidate.Role = RoleOwner

	return candidate
}

// validate resolves the Plex account behind an access token.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token to validate.
//
// Returns:
//   - account: The account behind the token, without a role.
//   - ok: False when Plex did not vouch for the token.
func (auth *Auth) validate(ctx context.Context, accessToken string) (User, bool) {
	valid, account, err := auth.clientFor(accessToken).ValidateToken(ctx)
	if err != nil || !valid || account == nil || account.ID == 0 {
		return User{}, false
	}

	username := account.Username
	if username == "" {
		username = account.Title
	}

	return User{PlexID: account.ID, Username: username, Role: ""}, true
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

// boundMachineID returns the machine identifier of the bound server. A binding
// stored before the identifier was recorded is asked for its identity, without
// a token, as a custom URL is.
//
// Parameters:
//   - ctx: Request context.
//   - client: Plex client used for the identity request.
//   - bound: The server bound now.
//
// Returns:
//   - machineID: The identifier, or empty when the server could not be asked.
func boundMachineID(ctx context.Context, client *plex.Client, bound plex.Server) string {
	if bound.MachineID != "" {
		return bound.MachineID
	}

	identityCtx, cancel := context.WithTimeout(ctx, identityTimeout)
	defer cancel()

	anonymous := bound

	anonymous.Token = ""

	identity, err := client.GetServerIdentity(identityCtx, anonymous)
	if err != nil {
		return ""
	}

	return identity.MachineIdentifier
}

// discoveredToken finds the token discovery reports for a server.
//
// Parameters:
//   - servers: Every connection discovery reported.
//   - machineID: The server's machine identifier.
//
// Returns:
//   - token: The server's token.
//   - found: False when no connection belongs to that server.
func discoveredToken(servers []plex.Server, machineID string) (string, bool) {
	for _, server := range servers {
		if server.MachineID == machineID {
			return server.Token, true
		}
	}

	return "", false
}
