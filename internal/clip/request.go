// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"time"
)

// Request is the export a caller asked for: the window, the profile, and the
// flags that shape the file. JSON names are the wire form of the same value.
type Request struct {
	Name          string  `json:"name"`
	MediaID       string  `json:"mediaId"       validate:"required"`
	MediaTitle    string  `json:"mediaTitle"    validate:"required"`
	MediaType     string  `json:"mediaType"`
	StartTime     float64 `json:"startTime"     validate:"gte=0"`
	Duration      float64 `json:"duration"      validate:"gt=0,lte=600"`
	Quality       string  `json:"quality"`
	ClipType      string  `json:"clipType"      validate:"oneof=clip video gif screenshot"`
	Width         int     `json:"width"`
	FPS           int     `json:"fps"`
	AudioIndex    int     `json:"audioIndex"`
	CropBlackBars bool    `json:"cropBlackBars"`
	WebSafeColor  *bool   `json:"webSafeColor"`
	PreserveHDR   *bool   `json:"preserveHdr"`
}

// Response is a clip as a caller reads it back. Input and output paths are
// left empty by the catalog mapping that fills this from a stored clip.
type Response struct {
	ID            string    `json:"id"`
	Name          string    `json:"name,omitempty"`
	MediaID       string    `json:"mediaId"`
	MediaTitle    string    `json:"mediaTitle"`
	MediaType     string    `json:"mediaType"`
	ClipType      Type      `json:"clipType"`
	Status        Status    `json:"status"`
	Progress      int       `json:"progress"`
	InputPath     string    `json:"inputPath"`
	OutputPath    string    `json:"outputPath,omitempty"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	AudioIndex    int       `json:"audioIndex"`
	CropBlackBars bool      `json:"cropBlackBars"`
	WebSafeColor  bool      `json:"webSafeColor"`
	PreserveHDR   bool      `json:"preserveHdr"`
}

// Flag reads an optional request flag, treating an absent one as false.
//
// Parameters:
//   - value: Submitted flag, nil when the request omitted the field.
//
// Returns:
//   - flag: The value the flag carries, false when it was absent.
func Flag(value *bool) bool {
	return value != nil && *value
}

// FlagOrDefault reads an optional request flag against a configured default.
//
// Parameters:
//   - value: Submitted flag, nil when the request omitted the field.
//   - fallback: Configured value to use when the field was absent.
//
// Returns:
//   - flag: The value the flag carries, otherwise fallback.
func FlagOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}

	return *value
}
