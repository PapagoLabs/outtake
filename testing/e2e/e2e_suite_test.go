// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

// Package e2e holds the bootstrap for the end-to-end suite.
//
// Each functional area lives in a subdirectory with its own package and its own
// test binary, because a compiled test binary may carry exactly one Ginkgo
// suite. This package is the suite entry point those areas hang off.
package e2e

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestE2E is the entry point the e2e suite runs under.
//
// Parameters:
//   - t: Test handle the suite reports through.
//
// Returns:
//   - nothing.
func TestE2E(t *testing.T) {
	t.Parallel()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Outtake E2E Suite")
}
