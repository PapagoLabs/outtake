// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectedLibraryID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		giveCurrent string
		giveQuery   string
		want        string
	}{
		{giveCurrent: "", giveQuery: "7", want: "7"},
		{giveCurrent: "http://localhost:8080/media?library=3", giveQuery: "", want: "3"},
		{giveCurrent: "http://localhost:8080/clips", giveQuery: "", want: ""},
		{giveCurrent: "://bad", giveQuery: "", want: ""},
		{giveCurrent: "http://localhost:8080/media?library=9", giveQuery: "1", want: "1"},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, selectedLibraryID(test.giveCurrent, test.giveQuery))
	}
}
