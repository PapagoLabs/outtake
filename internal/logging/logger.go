// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package logging provides structured logging for outtake.
package logging

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger is the global logger instance.
var Logger zerolog.Logger

// Init initializes the logger.
func Init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	if os.Getenv("ENV") == "development" {
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
