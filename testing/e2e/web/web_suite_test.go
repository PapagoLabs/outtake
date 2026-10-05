// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

// Package web holds the end-to-end specs for the HTML surface: every page the
// route table serves is fetched and checked for its marker, plus the health
// endpoint the deployment probes.
package web

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

// TestWeb is the entry point the page smoke specs run under.
//
// Parameters:
//   - t: Test handle the suite reports through.
//
// Returns:
//   - nothing.
func TestWeb(t *testing.T) {
	t.Parallel()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Outtake E2E Web Suite")
}
