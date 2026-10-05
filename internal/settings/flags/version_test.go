// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package flags

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionBind(t *testing.T) {
	t.Parallel()

	version := &Version{}
	flagSet := pflag.NewFlagSet("version", pflag.ContinueOnError)

	version.Bind(flagSet)

	jsonFlag := flagSet.Lookup(FlagJSON)
	require.NotNil(t, jsonFlag)
	assert.Equal(t, "false", jsonFlag.DefValue)
	assert.Equal(t, "Output version information in JSON format", jsonFlag.Usage)

	verboseFlag := flagSet.Lookup(FlagVerbose)
	require.NotNil(t, verboseFlag)
	assert.Equal(t, "false", verboseFlag.DefValue)
	assert.Equal(t, "Output detailed version information", verboseFlag.Usage)

	assert.False(t, version.JSON)
	assert.False(t, version.Verbose)
}

func TestVersionBindWritesTheParsedValues(t *testing.T) {
	t.Parallel()

	version := &Version{}
	flagSet := pflag.NewFlagSet("version", pflag.ContinueOnError)

	version.Bind(flagSet)
	require.NoError(t, flagSet.Parse([]string{"--" + FlagJSON, "--" + FlagVerbose + "=true"}))

	assert.True(t, version.JSON)
	assert.True(t, version.Verbose)
}

func TestVersionBindSetsOnlyThePassedFlag(t *testing.T) {
	t.Parallel()

	version := &Version{}
	flagSet := pflag.NewFlagSet("version", pflag.ContinueOnError)

	version.Bind(flagSet)
	require.NoError(t, flagSet.Parse([]string{"--" + FlagVerbose}))

	assert.False(t, version.JSON)
	assert.True(t, version.Verbose)
}

func TestVersionBindLeavesBothFlagsFalseWithoutParsing(t *testing.T) {
	t.Parallel()

	version := &Version{}
	flagSet := pflag.NewFlagSet("version", pflag.ContinueOnError)

	version.Bind(flagSet)
	require.NoError(t, flagSet.Parse(nil))

	assert.False(t, version.JSON)
	assert.False(t, version.Verbose)
}
