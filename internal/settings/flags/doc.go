// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package flags defines every command-line flag the outtake CLI accepts, one
// file per command. The commands under internal/cmd bind them and read the
// result; a flag that overrides configuration also applies that override to the
// loaded configuration, so no command repeats the rule.
package flags
