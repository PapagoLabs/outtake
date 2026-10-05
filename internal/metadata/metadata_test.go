// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package metadata

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreVersionState(t *testing.T) {
	t.Helper()

	version, commit, buildTime := Version, CommitSHA, BuildTime

	t.Cleanup(func() {
		Version, CommitSHA, BuildTime = version, commit, buildTime
		versionOnce = sync.Once{}
	})

	Version, CommitSHA, BuildTime = DefaultVersion, "", ""
	versionOnce = sync.Once{}
}

func versionInitRan() bool {
	fired := false

	versionOnce.Do(func() { fired = true })

	return fired
}

//nolint:paralleltest // mutates package-level version state.
func TestGetInfoRunsVersionInitOnce(t *testing.T) {
	restoreVersionState(t)

	GetInfo()

	assert.False(t, versionInitRan(), "GetInfo must initialize the version before reading it")
}

//nolint:paralleltest // mutates package-level version state.
func TestPrintJSONRunsVersionInitOnce(t *testing.T) {
	restoreVersionState(t)

	var buf bytes.Buffer

	require.NoError(t, PrintJSON(&buf))

	assert.False(t, versionInitRan(),
		"the JSON printer must initialize the version before reading it")
}

//nolint:paralleltest // mutates package-level version state.
func TestPrintVerboseRunsVersionInitOnce(t *testing.T) {
	restoreVersionState(t)

	var buf bytes.Buffer

	require.NoError(t, PrintVerbose(&buf))

	assert.False(t, versionInitRan(),
		"the verbose printer must initialize the version before reading it")
}

//nolint:paralleltest // mutates package-level version state.
func TestGetInfoReportsPackageVars(t *testing.T) {
	restoreVersionState(t)

	ensureVersion()

	CommitSHA = "abc1234"
	BuildTime = "2026-01-02T03:04:05Z"

	info := GetInfo()

	assert.Equal(t, Name, info.Name)
	assert.Equal(t, Version, info.Version)
	assert.Equal(t, "abc1234", info.CommitSHA)
	assert.Equal(t, ConvertToLocal("2026-01-02T03:04:05Z"), info.BuildTime)
	assert.Equal(t, GetGoVersion(), info.GoVersion)
	assert.Equal(t, runtime.GOOS, info.OS)
	assert.Equal(t, runtime.GOARCH, info.Arch)
}

//nolint:paralleltest // mutates package-level version state.
func TestGetInfoOmitsEmptyBuildVars(t *testing.T) {
	restoreVersionState(t)

	CommitSHA = ""
	BuildTime = ""

	info := GetInfo()

	assert.Empty(t, info.CommitSHA)
	assert.Empty(t, info.BuildTime)
}

//nolint:paralleltest // mutates package-level version state.
func TestPrintJSONReportsInitializedVersion(t *testing.T) {
	restoreVersionState(t)

	ensureVersion()

	var buf bytes.Buffer

	require.NoError(t, PrintJSON(&buf))

	assert.Contains(t, buf.String(), `"version": `+strconv.Quote(Version))
}

//nolint:paralleltest // mutates package-level version state.
func TestPrintVerboseReportsInitializedVersion(t *testing.T) {
	restoreVersionState(t)

	ensureVersion()

	var buf bytes.Buffer

	require.NoError(t, PrintVerbose(&buf))

	assert.Contains(t, buf.String(), "version:   "+Version)
}

//nolint:paralleltest // mutates package-level version state.
func TestPrintDefaultReportsInitializedVersion(t *testing.T) {
	restoreVersionState(t)

	ensureVersion()

	var buf bytes.Buffer

	require.NoError(t, PrintDefault(&buf))

	assert.Equal(t, Name+" "+String()+"\n", buf.String())
}

//nolint:paralleltest // mutates package-level version state.
func TestStringAppendsCommitSHA(t *testing.T) {
	restoreVersionState(t)

	ensureVersion()

	CommitSHA = "abc1234"

	assert.Equal(t, Version+" (abc1234)", String())
}

//nolint:paralleltest // mutates package-level version state.
func TestStringIsInitialized(t *testing.T) {
	restoreVersionState(t)

	ensureVersion()

	assert.Equal(t, Version, String())
	assert.False(t, versionInitRan())
}
