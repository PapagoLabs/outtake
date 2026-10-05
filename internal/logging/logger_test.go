// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package logging

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/settings/config"
)

func TestInit_DefaultLevel(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("LOG_LEVEL", "")

	Init()

	assert.Equal(t, zerolog.InfoLevel, Logger.GetLevel())
}

func TestInit_DevelopmentMode(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("LOG_LEVEL", "debug")

	Init()

	assert.Equal(t, zerolog.DebugLevel, Logger.GetLevel())
}

func TestInit_InvalidLevel(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("LOG_LEVEL", "invalid")

	Init()

	assert.Equal(t, zerolog.InfoLevel, Logger.GetLevel())
}

func TestInit_WritesToWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	logger := zerolog.New(&buf)
	logger.Info().Msg("test message")

	assert.Contains(t, buf.String(), "test message")
}

func TestInitFromConfig_ConfiguredLevelBeatsEnvironment(t *testing.T) {
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("ENV", "production")

	InitFromConfig(&config.Config{LogLevel: "debug", Env: "production"})

	assert.Equal(t, zerolog.DebugLevel, Logger.GetLevel())
}

//nolint:paralleltest // Replaces the package-level logger, which is shared state.
func TestInitFromConfig_EmptyLevelIsInfo(t *testing.T) {
	InitFromConfig(&config.Config{})

	assert.Equal(t, zerolog.InfoLevel, Logger.GetLevel())
}

//nolint:paralleltest // Replaces the package-level logger, which is shared state.
func TestInitFromConfig_InvalidLevelIsInfo(t *testing.T) {
	InitFromConfig(&config.Config{LogLevel: "invalid", Env: "production"})

	assert.Equal(t, zerolog.InfoLevel, Logger.GetLevel())
}

func TestInitFromConfig_NilConfigReadsEnvironment(t *testing.T) {
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("ENV", "production")

	InitFromConfig(nil)

	assert.Equal(t, zerolog.WarnLevel, Logger.GetLevel())
}

//nolint:paralleltest // Swaps os.Stderr, which is process-wide.
func TestInitFromConfig_DevelopmentWritesConsole(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	original := os.Stderr

	os.Stderr = writer

	t.Cleanup(func() { os.Stderr = original })

	InitFromConfig(&config.Config{LogLevel: "info", Env: "development"})

	Logger.Info().Msg("console message")

	require.NoError(t, writer.Close())

	out, readErr := io.ReadAll(reader)
	require.NoError(t, readErr)

	assert.Contains(t, string(out), "INF")
	assert.Contains(t, string(out), "console message")
	assert.NotContains(t, string(out), `"level":"info"`)
}

//nolint:paralleltest // Swaps os.Stderr, which is process-wide.
func TestInitFromConfig_ProductionWritesJSON(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	original := os.Stderr

	os.Stderr = writer

	t.Cleanup(func() { os.Stderr = original })

	InitFromConfig(&config.Config{LogLevel: "info", Env: "production"})

	Logger.Info().Msg("json message")

	require.NoError(t, writer.Close())

	out, readErr := io.ReadAll(reader)
	require.NoError(t, readErr)

	assert.Contains(t, string(out), `"level":"info"`)
	assert.Contains(t, string(out), "json message")
}
