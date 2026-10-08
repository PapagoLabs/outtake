// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestKeepHDRResolvesTheLegacyField covers callers that still send
// webSafeColor: it reads as the inverse of preserveHdr, and preserveHdr wins
// when both are sent.
func TestKeepHDRResolvesTheLegacyField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		preserve *bool
		webSafe  *bool
		want     *bool
	}{
		{name: "neither", preserve: nil, webSafe: nil, want: nil},
		{name: "keep", preserve: new(true), webSafe: nil, want: new(true)},
		{name: "tone map", preserve: new(false), webSafe: nil, want: new(false)},
		{name: "legacy web-safe on", preserve: nil, webSafe: new(true), want: new(false)},
		{name: "legacy web-safe off", preserve: nil, webSafe: new(false), want: new(true)},
		{name: "both", preserve: new(true), webSafe: new(true), want: new(true)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			req := Request{PreserveHDR: test.preserve, WebSafeColor: test.webSafe}
			edit := EditRequest{PreserveHDR: test.preserve, WebSafeColor: test.webSafe}

			assert.Equal(t, test.want, req.KeepHDR())
			assert.Equal(t, test.want, edit.KeepHDR())
		})
	}
}
