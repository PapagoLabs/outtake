// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package key

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path string
		want string
	}{
		"bare identifier":       {path: "/library/metadata/42", want: "42"},
		"trailing slash":        {path: "/library/metadata/42/", want: "42"},
		"children route":        {path: "/library/metadata/42/children", want: "42"},
		"nested children route": {path: "/library/metadata/42/children/43", want: "42"},
		"hubs path":             {path: "/hubs/metadata/42", want: ""},
		"prefix alone":          {path: "/library/metadata/", want: ""},
		"empty path":            {path: "", want: ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, ID(test.path))
		})
	}
}

func TestPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/library/metadata/42/children", Prefix+"42/children")
}
