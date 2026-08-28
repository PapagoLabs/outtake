// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package logging

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
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
