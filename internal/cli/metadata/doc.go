// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package metadata provides application version and build information.
// It uses Go build information (debug.ReadBuildInfo) to populate version
// at runtime, and can be extended with build-time flags for commit SHA
// and build timestamp.
//
// # Usage
//
//	import "github.com/PapagoLabs/outtake/internal/cli/metadata"
//	fmt.Printf("%s %s\n", metadata.Name, metadata.String())
//	info := metadata.GetInfo() // structured data for JSON/output
//
// # Build-time Injection
//
// To set Version, CommitSHA, and BuildTime at build time, use ldflags:
//
//	go build -ldflags "-X 'github.com/PapagoLabs/outtake/internal/cli/metadata.Version=v1.0.0' \
//		-X 'github.com/PapagoLabs/outtake/internal/cli/metadata.CommitSHA=abc123' \
//		-X 'github.com/PapagoLabs/outtake/internal/cli/metadata.BuildTime=2025-01-15T12:00:00Z'"
package metadata
