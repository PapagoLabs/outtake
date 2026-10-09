// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"github.com/rs/zerolog/log"
)

// retiredSetting is an environment variable that no longer configures
// anything, with what took its place.
type retiredSetting struct {
	// name is the environment variable.
	name string
	// replacement is where the setting now lives.
	replacement string
}

// profileKeepsHDR names the setting that replaced the global HDR switches.
const profileKeepsHDR = "the Keep HDR setting of each clip profile"

// retiredSettings lists the environment variables earlier releases read.
var retiredSettings = []retiredSetting{
	{name: "OUTTAKE_WEB_SAFE_COLOR", replacement: profileKeepsHDR},
	{name: "OUTTAKE_PRESERVE_HDR", replacement: profileKeepsHDR},
}

// setRetiredSettings lists the retired settings the environment still sets.
//
// Parameters:
//   - lookup: Reads an environment variable, such as [os.LookupEnv].
//
// Returns:
//   - set: The retired settings that are set, in listed order.
func setRetiredSettings(lookup func(string) (string, bool)) []retiredSetting {
	var set []retiredSetting

	for _, setting := range retiredSettings {
		if _, ok := lookup(setting.name); ok {
			set = append(set, setting)
		}
	}

	return set
}

// warnRetiredSettings warns about each retired setting the environment still
// sets, so an upgrade that silently changes behavior says why.
//
// Parameters:
//   - lookup: Reads an environment variable, such as [os.LookupEnv].
func warnRetiredSettings(lookup func(string) (string, bool)) {
	for _, setting := range setRetiredSettings(lookup) {
		log.Warn().
			Str("setting", setting.name).
			Str("replaced_by", setting.replacement).
			Msg("ignoring a retired setting")
	}
}
