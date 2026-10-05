// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

// Package plex holds the end-to-end specs for the Plex adapter. The specs that
// need a reachable server skip cleanly without credentials, and the specs that
// exercise pure functions run whatever the credentials are.
package plex

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

// testApp is the wired application this suite package drives.
var testApp = helpers.New()

var (
	_ = BeforeSuite(func() { testApp.BeforeSuite() })
	_ = AfterSuite(func() { testApp.AfterSuite() })
)

// TestPlex is the entry point the Plex specs run under.
//
// Parameters:
//   - t: Test handle the suite reports through.
//
// Returns:
//   - nothing.
func TestPlex(t *testing.T) {
	t.Parallel()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Outtake E2E Plex Suite")
}
