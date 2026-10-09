// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckHDRChoiceRefusesEitherField covers Keep HDR as a profile setting:
// a request or an edit that sends preserveHdr or webSafeColor, with any
// value, is refused, so a caller is never given a file it did not ask for.
func TestCheckHDRChoiceRefusesEitherField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		preserve *bool
		webSafe  *bool
		refused  bool
	}{
		{name: "neither", preserve: nil, webSafe: nil, refused: false},
		{name: "keep", preserve: new(true), webSafe: nil, refused: true},
		{name: "tone map", preserve: new(false), webSafe: nil, refused: true},
		{name: "web-safe on", preserve: nil, webSafe: new(true), refused: true},
		{name: "web-safe off", preserve: nil, webSafe: new(false), refused: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			req := Request{PreserveHDR: test.preserve, WebSafeColor: test.webSafe}
			edit := EditRequest{PreserveHDR: test.preserve, WebSafeColor: test.webSafe}

			if test.refused {
				require.ErrorIs(t, req.CheckHDRChoice(), ErrHDRIsAProfileSetting)
				assert.ErrorIs(t, edit.CheckHDRChoice(), ErrHDRIsAProfileSetting)

				return
			}

			assert.NoError(t, req.CheckHDRChoice())
			assert.NoError(t, edit.CheckHDRChoice())
		})
	}
}
