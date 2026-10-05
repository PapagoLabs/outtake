// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

// Package profiles holds the end-to-end specs for the clip profile settings
// page: listing, creating, marking a default, and deleting.
package profiles

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

// TestProfiles is the entry point the profile specs run under.
//
// Parameters:
//   - t: Test handle the suite reports through.
//
// Returns:
//   - nothing.
func TestProfiles(t *testing.T) {
	t.Parallel()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Outtake E2E Profiles Suite")
}
