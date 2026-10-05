// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

// Package previews holds the end-to-end specs for the segment preview flow: the
// request that submits a render, the status it is polled through, and the file
// the finished preview is served from.
package previews

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

// TestPreviews is the entry point the preview specs run under.
//
// Parameters:
//   - t: Test handle the suite reports through.
//
// Returns:
//   - nothing.
func TestPreviews(t *testing.T) {
	t.Parallel()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Outtake E2E Previews Suite")
}
