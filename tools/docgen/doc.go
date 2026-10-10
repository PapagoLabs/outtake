// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command docgen writes the outtake CLI reference as Hugo pages.
//
// It reads the command tree from [cmd.DocRoot], which builds the same tree the
// binary runs without loading any configuration. No command is executed, so
// docgen never touches the network, the database, or Plex, and the reference
// cannot drift from the shipped command line.
//
// Commands and flags are visited in sorted order, so the same tree always
// produces the same bytes.
//
// Usage:
//
//	go run ./tools/docgen -out ./docs/content/cli-reference
package main
