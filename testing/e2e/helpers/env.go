// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package helpers

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/onsi/ginkgo/v2"
)

// envAssignment is one key and value pair read out of the environment file.
type envAssignment struct {
	key   string
	value string
}

// EnvFile is what loading the e2e environment file produced.
type EnvFile struct {
	// Path is the file the keys were read from, empty when none was found.
	Path string
	// Loaded is the keys the file supplied to the process environment.
	Loaded []string
	// Kept is the keys the file carried that the process environment already
	// held, or that the file left empty.
	Kept []string
}

// The environment variables the e2e suite reads.
const (
	// TokenEnv carries the Plex access token.
	TokenEnv = "PLEX_TOKEN"

	// ServerURLEnv carries the Plex server URL.
	ServerURLEnv = "PLEX_SERVER_URL"

	// PathEnv overrides where the environment file is read from.
	PathEnv = "OUTTAKE_E2E_ENV"
)

// The layout of the environment file the suite loads.
const (
	// fileName is the environment file name.
	fileName = ".env"

	// dirName is the directory the file lives in.
	dirName = "e2e"

	// parentDir is the directory that holds the e2e tree. Requiring both halves
	// is what keeps the search from ever reaching the repository root .env,
	// which holds the OUTTAKE_* application configuration rather than the PLEX_*
	// credentials the suite reads.
	parentDir = "testing"

	// exportPrefix is the optional shell prefix an assignment may carry.
	exportPrefix = "export "

	// inlineComment is the whitespace-then-hash that starts a trailing comment.
	inlineComment = " #"

	// quotePairLen is the shortest a quoted value can be, an empty pair.
	quotePairLen = 2
)

// LoadEnv reads the e2e environment file into the process environment.
//
// Only testing/e2e/.env is read. A key the process environment already holds is
// left alone, so an explicit shell export always wins over the file. A key the
// file carries with an empty value is treated as absent, so a blanked-out line
// cannot pass for a credential.
//
// Returns:
//   - env: The keys the file supplied and the keys it left alone.
//   - err: Wrapped error when the file cannot be located, read, or applied.
func LoadEnv() (EnvFile, error) {
	path, err := envPath()
	if err != nil {
		return EnvFile{}, fmt.Errorf("locate the e2e environment file: %w", err)
	}

	if path == "" {
		return EnvFile{}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return EnvFile{}, fmt.Errorf("open the e2e environment file: %w", err)
	}
	defer func() { _ = file.Close() }()

	env, err := scanEnvFile(file)
	if err != nil {
		return EnvFile{}, fmt.Errorf("apply the e2e environment file: %w", err)
	}

	env.Path = path

	slices.Sort(env.Loaded)
	slices.Sort(env.Kept)

	return env, nil
}

// Report writes the loaded key names to the Ginkgo writer, which is what a
// verbose run prints. Values are never written, because they carry a Plex token.
//
// Returns:
//   - nothing.
func (env EnvFile) Report() {
	ginkgo.GinkgoWriter.Printf("[harness] e2e environment file: %s\n", EnvLocation())
	ginkgo.GinkgoWriter.Printf("[harness] keys taken from the file: %s\n", describe(env.Loaded))
	ginkgo.GinkgoWriter.Printf(
		"[harness] keys already exported and left alone: %s\n",
		describe(env.Kept),
	)
}

// EnvLocation names the environment file the suite reads.
//
// Returns:
//   - path: The resolved file path, or the canonical path when none was found.
func EnvLocation() string {
	if override := envPathOverride(); override != "" {
		return override
	}

	return filepath.Join(parentDir, dirName, fileName)
}

// scanEnvFile applies every assignment in the file, skipping the ones the
// process environment already holds and the ones the file left empty.
//
// Parameters:
//   - reader: The open environment file.
//
// Returns:
//   - env: The keys the file supplied and the keys it left alone.
//   - err: Wrapped error when the file cannot be read or a key cannot be set.
func scanEnvFile(reader io.Reader) (EnvFile, error) {
	env := EnvFile{}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		err := applyEnvLine(&env, scanner.Text())
		if err != nil {
			return EnvFile{}, fmt.Errorf("apply one line: %w", err)
		}
	}

	err := scanner.Err()
	if err != nil {
		return EnvFile{}, fmt.Errorf("read the e2e environment file: %w", err)
	}

	return env, nil
}

// applyEnvLine applies one line's assignment to the process environment and
// records which side of the split the key fell on. A line that carries no
// usable assignment is skipped.
//
// Parameters:
//   - env: Running tally the key is recorded on.
//   - line: One raw line from the environment file.
//
// Returns:
//   - err: Wrapped error when the key cannot be set.
func applyEnvLine(env *EnvFile, line string) error {
	parsed, ok := parseEnvLine(line)
	if !ok {
		return nil
	}

	if parsed.value == "" || isSet(parsed.key) {
		env.Kept = append(env.Kept, parsed.key)

		return nil
	}

	err := os.Setenv(parsed.key, parsed.value)
	if err != nil {
		return fmt.Errorf("set %s from the e2e environment file: %w", parsed.key, err)
	}

	env.Loaded = append(env.Loaded, parsed.key)

	return nil
}

// envPathOverride reads the explicit environment file location, if one is set.
//
// Returns:
//   - path: The configured location, empty when none is set.
func envPathOverride() string {
	return strings.TrimSpace(os.Getenv(PathEnv))
}

// envPath resolves the environment file. The override wins when it is set;
// otherwise the search walks up from the working directory, which is the package
// directory go test runs each test binary in, and stops at the first directory
// named testing/e2e.
//
// Returns:
//   - path: The resolved file path, empty when no such directory was found.
//   - err: Wrapped error when the working directory cannot be read.
func envPath() (string, error) {
	if override := envPathOverride(); override != "" {
		return override, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve the working directory: %w", err)
	}

	for dir != filepath.Dir(dir) {
		if filepath.Base(dir) == dirName && filepath.Base(filepath.Dir(dir)) == parentDir {
			return filepath.Join(dir, fileName), nil
		}

		dir = filepath.Dir(dir)
	}

	return "", nil
}

// isSet reports whether the process environment already carries a key.
//
// Parameters:
//   - key: Environment variable name.
//
// Returns:
//   - found: True when the variable is present, whatever its value.
func isSet(key string) bool {
	_, found := os.LookupEnv(key)

	return found
}

// parseEnvLine reads one assignment, tolerating an export prefix, surrounding
// quotes, and a trailing comment. Blank lines and comments yield no assignment.
//
// Parameters:
//   - line: One raw line from the environment file.
//
// Returns:
//   - parsed: The key and the value it was assigned, quotes and comments removed.
//   - ok: False when the line carries no usable assignment.
func parseEnvLine(line string) (envAssignment, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return envAssignment{}, false
	}

	assignment := strings.TrimPrefix(trimmed, exportPrefix)

	key, raw, found := strings.Cut(assignment, "=")
	if !found {
		return envAssignment{}, false
	}

	key = strings.TrimSpace(key)
	if !validKey(key) {
		return envAssignment{}, false
	}

	return envAssignment{key: key, value: unquote(raw)}, true
}

// unquote strips a surrounding quote pair, or otherwise drops a trailing
// comment. A quoted value is taken verbatim, so a hash inside quotes survives.
//
// Parameters:
//   - raw: The right-hand side of an assignment, quotes and comments included.
//
// Returns:
//   - value: The value the assignment carries.
func unquote(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) < quotePairLen {
		return value
	}

	first, last := value[0], value[len(value)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return value[1 : len(value)-1]
	}

	if before, _, found := strings.Cut(value, inlineComment); found {
		return strings.TrimSpace(before)
	}

	return value
}

// validKey reports whether a name is shaped like an environment variable.
//
// Parameters:
//   - key: Candidate environment variable name.
//
// Returns:
//   - ok: True when the name is usable.
func validKey(key string) bool {
	if key == "" {
		return false
	}

	for index, char := range key {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char == '_':
		case char >= '0' && char <= '9' && index > 0:
		default:
			return false
		}
	}

	return true
}

// describe renders a key list for the Ginkgo writer.
//
// Parameters:
//   - keys: Key names to render.
//
// Returns:
//   - text: The joined names, or "none" when the list is empty.
func describe(keys []string) string {
	if len(keys) == 0 {
		return "none"
	}

	return strings.Join(keys, ", ")
}
