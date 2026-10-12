// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package users

import (
	"math"
	"strconv"
	"strings"
	"time"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/quota"
)

// refusal is a limits form's refusal, worded for the page.
type refusal string

// Numbers the form is read with.
const (
	// bytesPerGB is how many bytes the form's gigabyte holds.
	bytesPerGB = 1_000_000_000
	// floatBits is the precision a storage field is read at.
	floatBits = 64
)

// Limit form fields.
const (
	// fieldActive is the concurrent renders.
	fieldActive = "active"
	// fieldWeekly is the renders per week.
	fieldWeekly = "weekly"
	// fieldStorage is the storage in gigabytes.
	fieldStorage = "storageGB"
	// fieldClip is the longest clip in seconds.
	fieldClip = "clipSeconds"
)

// Refusals of a limits form.
const (
	// errActive refuses a concurrent renders value.
	errActive refusal = "Enter 0 or a whole number above it for Concurrent Renders"
	// errWeekly refuses a renders per week value.
	errWeekly refusal = "Enter 0 or a whole number above it for Renders per Week"
	// errStorage refuses a storage value.
	errStorage refusal = "Enter 0 or a number above it for Storage"
	// errClip refuses a longest clip value.
	errClip refusal = "Enter 0 or a whole number above it for Longest Clip"
)

// parseGlobalLimits reads the global limits form.
//
// An empty field sets no limit.
//
// Parameters:
//   - ctx: Form post carrying the limits.
//
// Returns:
//   - limits: The limits.
//   - err: The refusal of the first field that is not a valid limit.
func parseGlobalLimits(ctx fiber.Ctx) (quota.Limits, error) {
	override, err := parseOverride(ctx)
	if err != nil {
		//nolint:wrapcheck // The refusal is the page's message as it is.
		return quota.Limits{}, err
	}

	return override.Apply(quota.Limits{}), nil
}

// parseOverride reads one user's limits form.
//
// An empty field leaves that limit to the global one.
//
// Parameters:
//   - ctx: Form post carrying the limits.
//
// Returns:
//   - override: The limits.
//   - err: The refusal of the first field that is not a valid limit.
func parseOverride(ctx fiber.Ctx) (quota.Override, error) {
	override := quota.Override{ClipLength: nil, Storage: nil, Active: nil, Weekly: nil}

	for _, field := range []struct {
		apply   func(number int)
		name    string
		refusal refusal
	}{
		{name: fieldActive, refusal: errActive, apply: func(number int) { override.Active = &number }},
		{name: fieldWeekly, refusal: errWeekly, apply: func(number int) { override.Weekly = &number }},
		{
			name:    fieldClip,
			refusal: errClip,
			apply: func(number int) {
				override.ClipLength = new(time.Duration(number) * time.Second)
			},
		},
	} {
		number, set, err := wholeField(ctx.FormValue(field.name), field.refusal)
		if err != nil {
			//nolint:wrapcheck // The refusal is the page's message as it is.
			return quota.Override{}, err
		}

		if set {
			field.apply(number)
		}
	}

	bytes, set, err := storageField(ctx.FormValue(fieldStorage))
	if err != nil {
		//nolint:wrapcheck // The refusal is the page's message as it is.
		return quota.Override{}, err
	}

	if set {
		override.Storage = &bytes
	}

	return override, nil
}

// Error returns the refusal's text.
//
// Returns:
//   - message: What the page shows.
func (message refusal) Error() string {
	return string(message)
}

// storageField reads a storage field in gigabytes.
//
// Parameters:
//   - value: The field's text.
//
// Returns:
//   - bytes: The storage in bytes.
//   - set: False for an empty field.
//   - err: errStorage for anything but 0 or a number above it.
//
//nolint:nonamedreturns // The named results tell the value from its flag.
func storageField(value string) (bytes int64, set bool, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, nil
	}

	gigabytes, parseErr := strconv.ParseFloat(value, floatBits)
	if parseErr != nil || gigabytes < 0 || math.IsNaN(gigabytes) ||
		gigabytes > math.MaxInt64/bytesPerGB {

		return 0, false, errStorage
	}

	return int64(math.Round(gigabytes * bytesPerGB)), true, nil
}

// wholeField reads a whole-number field.
//
// Parameters:
//   - value: The field's text.
//   - refused: What a value that is not a whole number of 0 or more reads as.
//
// Returns:
//   - number: The number.
//   - set: False for an empty field.
//   - err: The refusal for an invalid value.
//
//nolint:nonamedreturns // The named results tell the value from its flag.
func wholeField(value string, refused refusal) (number int, set bool, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, nil
	}

	number, parseErr := strconv.Atoi(value)
	if parseErr != nil || number < 0 || number > math.MaxInt32 {
		return 0, false, refused
	}

	return number, true, nil
}
