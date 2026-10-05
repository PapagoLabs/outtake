// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package logging provides structured logging for outtake.
package logging

import (
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/settings/config"
)

const (
	// defaultLevel is used when no level is configured.
	defaultLevel = "info"

	// developmentEnv selects the console writer.
	developmentEnv = "development"
)

// Logger is the global logger instance.
var Logger zerolog.Logger

// initMu serializes writes to the logger globals.
var initMu sync.Mutex

// Init initializes the logger from the process environment.
func Init() {
	initLogger(os.Getenv("LOG_LEVEL"), os.Getenv("ENV"))
}

// InitFromConfig initializes the logger from the loaded configuration.
//
// Parameters:
//   - cfg: The loaded configuration supplying LogLevel and Env.
func InitFromConfig(cfg *config.Config) {
	if cfg == nil {
		Init()

		return
	}

	initLogger(cfg.LogLevel, cfg.Env)
}

// initLogger builds the global logger and installs it as zerolog's default.
//
// Parameters:
//   - logLevel: Configured level name. An empty or unrecognized value logs at info.
//   - env: Configured environment. developmentEnv selects a console writer.
func initLogger(logLevel, env string) {
	initMu.Lock()
	defer initMu.Unlock()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if logLevel == "" {
		logLevel = defaultLevel
	}

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	if env == developmentEnv {
		Logger = zerolog.New(zerolog.ConsoleWriter{
			Out:                   os.Stderr,
			NoColor:               false,
			TimeFormat:            time.RFC3339,
			TimeLocation:          nil,
			PartsOrder:            nil,
			PartsExclude:          nil,
			FieldsOrder:           nil,
			FieldsExclude:         nil,
			FormatTimestamp:       nil,
			FormatLevel:           nil,
			FormatCaller:          nil,
			FormatMessage:         nil,
			FormatFieldName:       nil,
			FormatFieldValue:      nil,
			FormatErrFieldName:    nil,
			FormatErrFieldValue:   nil,
			FormatPartValueByName: nil,
			FormatExtra:           nil,
			FormatPrepare:         nil,
		}).With().Timestamp().Caller().Logger().Level(level)
	} else {
		Logger = zerolog.New(os.Stderr).
			With().
			Timestamp().
			Logger().
			Level(level)
	}

	log.Logger = Logger
}
