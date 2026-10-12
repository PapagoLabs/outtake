// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"strconv"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip/quota"
)

// UserItem is one user as the users page shows them.
type UserItem struct {
	// Limits are the user's own limits as form text, empty where the user
	// takes the limit every member has.
	Limits LimitFields
	// Username is the Plex username.
	Username string
	// Role is the role's name, such as "Member".
	Role string
	// LastLogin is when the user last logged in, such as "Oct 12, 2026".
	LastLogin string
	// Usage is what the user's clips take, such as "3 renders this week ·
	// 1.2 GB".
	Usage string
	// PlexID is the plex.tv account id the forms post.
	PlexID int
	// Pending is true for an account waiting for approval.
	Pending bool
	// CanApprove shows Approve and Reject.
	CanApprove bool
	// CanRemove shows Remove.
	CanRemove bool
	// CanLimit shows the user's limits form.
	CanLimit bool
	// CanMakeAdmin shows Make Admin.
	CanMakeAdmin bool
	// CanMakeMember shows Make Member.
	CanMakeMember bool
}

// LimitFields are a limits form's values as text.
type LimitFields struct {
	// Active is the concurrent renders.
	Active string
	// Weekly is the renders per week.
	Weekly string
	// StorageGB is the storage in gigabytes.
	StorageGB string
	// ClipSeconds is the longest clip in seconds.
	ClipSeconds string
}

// Placeholders of an empty limit field.
const (
	// PlaceholderNoLimit names a limit that is off.
	PlaceholderNoLimit = "No limit"
	// PlaceholderClipMaximum names a longest clip left to the installation.
	PlaceholderClipMaximum = "Installation maximum"
)

// Numbers a limit is written in.
const (
	// bytesPerGB is how many bytes the limits form's gigabyte holds.
	bytesPerGB = 1_000_000_000
	// floatBits is the precision a storage limit is written at.
	floatBits = 64
)

// GlobalLimitFields fills the global limits form.
//
// A limit that is off is left empty, which its placeholder names.
//
// Parameters:
//   - limits: The limits every member has.
//
// Returns:
//   - fields: Each limit as text, empty where there is no limit.
func GlobalLimitFields(limits quota.Limits) LimitFields {
	return LimitFields{
		Active:      wholeOrEmpty(int64(limits.Active)),
		Weekly:      wholeOrEmpty(int64(limits.Weekly)),
		StorageGB:   gigabytesOrEmpty(limits.Storage),
		ClipSeconds: wholeOrEmpty(int64(limits.ClipLength / time.Second)),
	}
}

// LimitPlaceholders names what each empty limit field stands for.
//
// Parameters:
//   - global: The global limits as their form shows them, all empty for the
//     global form itself.
//
// Returns:
//   - placeholders: Each global value, or what no limit means for that field.
func LimitPlaceholders(global LimitFields) LimitFields {
	return LimitFields{
		Active:      textOr(global.Active, PlaceholderNoLimit),
		Weekly:      textOr(global.Weekly, PlaceholderNoLimit),
		StorageGB:   textOr(global.StorageGB, PlaceholderNoLimit),
		ClipSeconds: textOr(global.ClipSeconds, PlaceholderClipMaximum),
	}
}

// OverrideFields fills one user's limits form.
//
// Parameters:
//   - override: The user's own limits.
//
// Returns:
//   - fields: Each limit as text, empty where the user takes the global one.
func OverrideFields(override quota.Override) LimitFields {
	fields := LimitFields{Active: "", Weekly: "", StorageGB: "", ClipSeconds: ""}

	if override.Active != nil {
		fields.Active = strconv.Itoa(*override.Active)
	}

	if override.Weekly != nil {
		fields.Weekly = strconv.Itoa(*override.Weekly)
	}

	if override.Storage != nil {
		fields.StorageGB = gigabytes(*override.Storage)
	}

	if override.ClipLength != nil {
		fields.ClipSeconds = strconv.FormatInt(int64(*override.ClipLength/time.Second), decimalBase)
	}

	return fields
}

// UsageLabel describes what a user's clips take.
//
// Parameters:
//   - renders: Renders started in the last seven days.
//   - bytes: Bytes the user's clips take.
//
// Returns:
//   - label: Such as "3 renders this week · 1.2 GB".
func UsageLabel(renders int, bytes int64) string {
	noun := " renders"
	if renders == 1 {
		noun = " render"
	}

	return strconv.Itoa(renders) + noun + " this week · " + FormatBytes(uint64(max(bytes, 0)))
}

// gigabytes writes a byte count in gigabytes, without trailing zeros.
//
// Parameters:
//   - bytes: The byte count.
//
// Returns:
//   - text: Such as "25" or "2.5".
func gigabytes(bytes int64) string {
	return strconv.FormatFloat(float64(bytes)/bytesPerGB, 'f', -1, floatBits)
}

// gigabytesOrEmpty writes a storage limit in gigabytes, empty for none.
//
// Parameters:
//   - bytes: The limit in bytes.
//
// Returns:
//   - text: The gigabytes, or empty for 0.
func gigabytesOrEmpty(bytes int64) string {
	if bytes <= 0 {
		return ""
	}

	return gigabytes(bytes)
}

// textOr returns a value, or a fallback for an empty one.
//
// Parameters:
//   - value: The value.
//   - fallback: What an empty value reads as.
//
// Returns:
//   - text: The value or the fallback.
func textOr(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

// wholeOrEmpty writes a whole-number limit, empty for none.
//
// Parameters:
//   - number: The limit.
//
// Returns:
//   - text: The number, or empty for 0.
func wholeOrEmpty(number int64) string {
	if number <= 0 {
		return ""
	}

	return strconv.FormatInt(number, decimalBase)
}
