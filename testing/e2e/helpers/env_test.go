// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package helpers

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvPathResolvesTheSuiteFile checks that the walk lands on the e2e
// environment file from the harness package directory, which is the working
// directory `go test` runs this binary in. It resolves the path only, so no
// environment file is opened.
func TestEnvPathResolvesTheSuiteFile(t *testing.T) {
	t.Parallel()

	path, err := envPath()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join("testing", dirName, fileName), lastThree(path))
}

// TestEnvPathHonoursTheOverride checks that an explicit override wins over the
// walk, which is what lets a run point at a file outside the tree.
func TestEnvPathHonoursTheOverride(t *testing.T) {
	t.Setenv(PathEnv, "/tmp/outtake-e2e-override.env")

	path, err := envPath()
	require.NoError(t, err)

	assert.Equal(t, "/tmp/outtake-e2e-override.env", path)
}

// TestEnvPathNeverReachesTheRepositoryRoot checks the guard that keeps the
// search off the repository root .env: from anywhere, the walk only ever
// answers with a file inside a directory named testing/e2e.
func TestEnvPathNeverReachesTheRepositoryRoot(t *testing.T) {
	t.Parallel()

	path, err := envPath()
	require.NoError(t, err)

	assert.Equal(t, dirName, filepath.Base(filepath.Dir(path)))
	assert.Equal(t, parentDir, filepath.Base(filepath.Dir(filepath.Dir(path))))
}

// TestParseEnvLine covers the shapes an environment file is written in.
func TestParseEnvLine(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		line      string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		"plain assignment": {
			line:      "PLEX_TOKEN=abc123",
			wantKey:   "PLEX_TOKEN",
			wantValue: "abc123",
			wantOK:    true,
		},
		"export prefix": {
			line:      "export PLEX_TOKEN=abc123",
			wantKey:   "PLEX_TOKEN",
			wantValue: "abc123",
			wantOK:    true,
		},
		"indented export": {
			line:      "   export PLEX_TOKEN=abc123   ",
			wantKey:   "PLEX_TOKEN",
			wantValue: "abc123",
			wantOK:    true,
		},
		"double quoted value": {
			line:      `PLEX_SERVER_URL="https://plex.example.com"`,
			wantKey:   "PLEX_SERVER_URL",
			wantValue: "https://plex.example.com",
			wantOK:    true,
		},
		"single quoted value": {
			line:      "PLEX_SERVER_URL='https://plex.example.com'",
			wantKey:   "PLEX_SERVER_URL",
			wantValue: "https://plex.example.com",
			wantOK:    true,
		},
		"quoted value keeps an inner hash": {
			line:      `PLEX_SERVER_URL="https://plex.example.com/#hash"`,
			wantKey:   "PLEX_SERVER_URL",
			wantValue: "https://plex.example.com/#hash",
			wantOK:    true,
		},
		"unquoted value keeps an inner hash": {
			line:      "PLEX_SERVER_URL=https://plex.example.com/#hash",
			wantKey:   "PLEX_SERVER_URL",
			wantValue: "https://plex.example.com/#hash",
			wantOK:    true,
		},
		"trailing comment": {
			line:      "PLEX_TOKEN=abc123 # the account token",
			wantKey:   "PLEX_TOKEN",
			wantValue: "abc123",
			wantOK:    true,
		},
		"blank line": {
			line:   "   ",
			wantOK: false,
		},
		"comment line": {
			line:   "# PLEX_TOKEN=abc123",
			wantOK: false,
		},
		"indented comment line": {
			line:   "\t# a comment",
			wantOK: false,
		},
		"no separator": {
			line:   "PLEX_TOKEN",
			wantOK: false,
		},
		"key with a dash": {
			line:   "PLEX-TOKEN=abc123",
			wantOK: false,
		},
		"empty key": {
			line:   "=abc123",
			wantOK: false,
		},
		"key starting with a digit": {
			line:   "1TOKEN=abc123",
			wantOK: false,
		},
		"empty value": {
			line:      "PLEX_TOKEN=",
			wantKey:   "PLEX_TOKEN",
			wantValue: "",
			wantOK:    true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			parsed, ok := parseEnvLine(test.line)

			assert.Equal(t, test.wantOK, ok)
			assert.Equal(t, test.wantKey, parsed.key)
			assert.Equal(t, test.wantValue, parsed.value)
		})
	}
}

// TestEnvLocationNamesTheFile checks the path the skip reason names.
func TestEnvLocationNamesTheFile(t *testing.T) {
	assert.Equal(t, filepath.Join("testing", dirName, fileName), EnvLocation())

	t.Setenv(PathEnv, "/tmp/outtake-e2e-override.env")

	assert.Equal(t, "/tmp/outtake-e2e-override.env", EnvLocation())
}

// TestIsSetReadsTheProcessEnvironment covers the guard that keeps a shell export
// ahead of the file.
func TestIsSetReadsTheProcessEnvironment(t *testing.T) {
	const key = "OUTTAKE_E2E_IS_SET_PROBE"

	assert.False(t, isSet(key), "an absent variable is not exported")

	t.Setenv(key, "")

	assert.True(t, isSet(key), "an empty value still counts as exported")
}

// lastThree returns the trailing path segments, joined, for comparison against
// a relative path.
func lastThree(path string) string {
	return filepath.Join(
		filepath.Base(filepath.Dir(filepath.Dir(path))),
		filepath.Base(filepath.Dir(path)),
		filepath.Base(path),
	)
}
