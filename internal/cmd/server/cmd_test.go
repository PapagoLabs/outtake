// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/settings/flags"
)

func TestNewCommand(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "server", cmd.Use)
	assert.Equal(t, "Manage the outtake server", cmd.Short)
	assert.Nil(t, cmd.RunE)
	require.Len(t, cmd.Commands(), 1)
	assert.Equal(t, "start", cmd.Commands()[0].Use)
}

func TestNewCommandResolvesTheStartSubcommand(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	cmd := NewCommand()

	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cmd.SetArgs([]string{"start", "--help"})

	require.NoError(t, cmd.Execute())

	assert.Contains(t, buf.String(), "Start the outtake server")
	assert.Contains(t, buf.String(), flags.FlagListen)
}
