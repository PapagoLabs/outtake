// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package users

import (
	"context"
	"io"
	"strconv"
	"time"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/quota"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// Store keeps the users.
type Store interface {
	// ListUsers returns every user, the owner first.
	ListUsers(ctx context.Context) ([]database.UserRecord, error)
	// UserRole returns a user's role, pending for an account not approved.
	UserRole(ctx context.Context, plexUserID int) (string, bool, error)
	// ApproveUser lets a pending account in as a member.
	ApproveUser(ctx context.Context, plexUserID int) error
	// RejectUser deletes an account still waiting for approval.
	RejectUser(ctx context.Context, plexUserID int) error
	// RemoveUser deletes a user other than the owner.
	RemoveUser(ctx context.Context, plexUserID int) error
	// SetUserRole makes a user an admin or a member.
	SetUserRole(ctx context.Context, plexUserID int, role string) error
	// SaveUserLimits stores a user's own render limits.
	SaveUserLimits(ctx context.Context, plexUserID int, override quota.Override) error
}

// LimitStore keeps the global render limits and what users have rendered.
type LimitStore interface {
	// GlobalLimits returns the limits every member has.
	GlobalLimits(ctx context.Context) (quota.Limits, error)
	// SaveGlobalLimits stores the limits every member has.
	SaveGlobalLimits(ctx context.Context, limits quota.Limits) error
	// UsageByOwner returns every user's renders since a time and storage.
	UsageByOwner(ctx context.Context, since time.Time) (map[int]database.UserUsage, error)
}

// Handler serves the users settings page and the pending page.
type Handler struct {
	// store keeps the users.
	store Store
	// limits keeps the global limits and what users have rendered.
	limits LimitStore
	// now reads the clock.
	now func() time.Time
}

// Messages the users page shows.
const (
	// msgCannotChange is shown when a change is not the viewer's to make.
	msgCannotChange = "You can't make that change to this user"
	// dateLayout writes a sign-in date.
	dateLayout = "Jan 2, 2006"
)

// New creates the users handler.
//
// Parameters:
//   - store: Keeps the users.
//   - limits: Keeps the global limits and what users have rendered.
//
// Returns:
//   - handler: The users handler.
func New(store Store, limits LimitStore) *Handler {
	return &Handler{store: store, limits: limits, now: time.Now}
}

// Approve lets a pending account in as a member.
//
// Parameters:
//   - ctx: Request naming the account.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) Approve(ctx fiber.Ctx) error {
	//nolint:wrapcheck // change writes the response and wraps its own failures.
	return handler.change(ctx, identity.ActionApprove, handler.store.ApproveUser)
}

// Page renders the users, the accounts waiting for approval, and the limits
// every member has.
//
// Parameters:
//   - ctx: The request.
//
// Returns:
//   - err: Non-nil when the page cannot be written.
func (handler *Handler) Page(ctx fiber.Ctx) error {
	caller, _ := middleware.Caller(ctx)

	global, err := handler.limits.GlobalLimits(ctx.Context())
	if err != nil {
		respond.SetFlash(ctx, respond.Fail(ctx, err))

		global = quota.Defaults()
	}

	records, err := handler.store.ListUsers(ctx.Context())
	if err != nil {
		respond.SetFlash(ctx, respond.Fail(ctx, err))
	}

	// Usage that cannot be read is left nil, which leaves every usage line
	// empty.
	usage, usageErr := handler.limits.UsageByOwner(ctx.Context(), handler.now().Add(-quota.Window))
	if usageErr != nil {
		usage = nil
	}

	items := make([]view.UserItem, 0, len(records))
	for index := range records {
		items = append(items, userItem(caller, &records[index], usage))
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.Users(pages.UsersProps{
			Users:        items,
			Global:       view.GlobalLimitFields(global),
			CanSetGlobal: caller.Manages(),
		}).Render(ctx.Context(), writer)
	})
}

// Pending renders the page an account waits on until it is approved.
//
// An approved account is sent on to the dashboard.
//
// Parameters:
//   - ctx: The request.
//
// Returns:
//   - err: Non-nil when the page cannot be written.
func (*Handler) Pending(ctx fiber.Ctx) error {
	caller, _ := middleware.Caller(ctx)
	if caller.Role != identity.RolePending {
		return respond.RedirectTo(ctx, routes.PathRoot)
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.Pending().Render(ctx.Context(), writer)
	})
}

// Reject turns a pending account away.
//
// The account can ask again by logging in.
//
// Parameters:
//   - ctx: Request naming the account.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) Reject(ctx fiber.Ctx) error {
	//nolint:wrapcheck // change writes the response and wraps its own failures.
	return handler.change(ctx, identity.ActionReject, handler.store.RejectUser)
}

// Remove deletes an approved user, keeping their clips.
//
// Parameters:
//   - ctx: Request naming the user.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) Remove(ctx fiber.Ctx) error {
	//nolint:wrapcheck // change writes the response and wraps its own failures.
	return handler.change(ctx, identity.ActionRemove, handler.store.RemoveUser)
}

// SaveGlobalLimits stores the limits every member has.
//
// Parameters:
//   - ctx: Form post carrying the limits.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) SaveGlobalLimits(ctx fiber.Ctx) error {
	limits, err := parseGlobalLimits(ctx)
	if err != nil {
		respond.SetFlash(ctx, view.NewNotice(err.Error()))

		return respond.RedirectTo(ctx, routes.PathSettingsUsers)
	}

	err = handler.limits.SaveGlobalLimits(ctx.Context(), limits)
	if err != nil {
		respond.SetFlash(ctx, respond.Fail(ctx, err))
	}

	return respond.RedirectTo(ctx, routes.PathSettingsUsers)
}

// SaveUserLimits stores one user's own limits.
//
// Parameters:
//   - ctx: Form post naming the user and carrying the limits.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) SaveUserLimits(ctx fiber.Ctx) error {
	override, err := parseOverride(ctx)
	if err != nil {
		respond.SetFlash(ctx, view.NewNotice(err.Error()))

		return respond.RedirectTo(ctx, routes.PathSettingsUsers)
	}

	//nolint:wrapcheck // change writes the response and wraps its own failures.
	return handler.change(
		ctx,
		identity.ActionLimits,
		func(ctx context.Context, plexUserID int) error {
			return handler.store.SaveUserLimits(ctx, plexUserID, override)
		},
	)
}

// SetRole makes a user an admin or a member.
//
// Parameters:
//   - ctx: Form post naming the user and the role.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) SetRole(ctx fiber.Ctx) error {
	role := ctx.FormValue("role")

	action := identity.ActionMakeMember
	if role == string(identity.RoleAdmin) {
		action = identity.ActionMakeAdmin
	}

	//nolint:wrapcheck // change writes the response and wraps its own failures.
	return handler.change(ctx, action, func(ctx context.Context, plexUserID int) error {
		return handler.store.SetUserRole(ctx, plexUserID, role)
	})
}

// change makes one change to a user when the caller may make it.
//
// Parameters:
//   - ctx: Request naming the user.
//   - action: The change.
//   - apply: Makes the change.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) change(
	ctx fiber.Ctx,
	action identity.Action,
	apply func(ctx context.Context, plexUserID int) error,
) error {
	caller, _ := middleware.Caller(ctx)

	plexUserID, err := strconv.Atoi(ctx.Params(routes.ParamID))
	if err != nil {
		respond.SetFlash(ctx, view.NewNotice(msgCannotChange))

		return respond.RedirectTo(ctx, routes.PathSettingsUsers)
	}

	role, found, err := handler.store.UserRole(ctx.Context(), plexUserID)
	if err != nil {
		respond.SetFlash(ctx, respond.Fail(ctx, err))

		return respond.RedirectTo(ctx, routes.PathSettingsUsers)
	}

	if !found || !caller.May(action, identity.Role(role)) {
		respond.SetFlash(ctx, view.NewNotice(msgCannotChange))

		return respond.RedirectTo(ctx, routes.PathSettingsUsers)
	}

	err = apply(ctx.Context(), plexUserID)
	if err != nil {
		respond.SetFlash(ctx, respond.Fail(ctx, err))
	}

	return respond.RedirectTo(ctx, routes.PathSettingsUsers)
}

// userItem describes one user for the page, with the changes the caller may
// make to them.
//
// Parameters:
//   - caller: The viewer.
//   - record: The user.
//   - usage: Every user's usage by plex.tv id, nil when it could not be read,
//     which leaves the usage line empty.
//
// Returns:
//   - item: The page model.
func userItem(
	caller identity.User,
	record *database.UserRecord,
	usage map[int]database.UserUsage,
) view.UserItem {
	role := identity.Role(record.Role)

	item := view.UserItem{
		Limits:        view.OverrideFields(record.Limits),
		Username:      record.Username,
		Role:          roleName(role),
		LastLogin:     "",
		Usage:         "",
		PlexID:        record.PlexID,
		Pending:       role == identity.RolePending,
		CanApprove:    caller.May(identity.ActionApprove, role),
		CanRemove:     caller.May(identity.ActionRemove, role),
		CanLimit:      caller.May(identity.ActionLimits, role),
		CanMakeAdmin:  caller.May(identity.ActionMakeAdmin, role),
		CanMakeMember: caller.May(identity.ActionMakeMember, role),
	}

	if usage != nil {
		used := usage[record.PlexID]

		item.Usage = view.UsageLabel(used.Renders, used.Bytes)
	}

	if !record.LastLoginAt.IsZero() {
		item.LastLogin = record.LastLoginAt.Format(dateLayout)
	}

	return item
}

// roleName names a role on the page.
//
// Parameters:
//   - role: The role.
//
// Returns:
//   - name: Such as "Owner" or "Member".
func roleName(role identity.Role) string {
	switch role {
	case identity.RoleOwner:
		return "Owner"
	case identity.RoleAdmin:
		return "Admin"
	case identity.RolePending:
		return "Pending"
	default:
		return "Member"
	}
}
