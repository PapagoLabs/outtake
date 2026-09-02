// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package api provides types for the outtake HTTP API.
package api

import (
	"time"
)

// ClipRequest represents a request to create a clip.
type ClipRequest struct {
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
}

// ClipResponse represents the response for a clip job.
type ClipResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name,omitempty"`
	MediaID       string    `json:"mediaId"`
	MediaTitle    string    `json:"mediaTitle"`
	MediaType     string    `json:"mediaType"`
	ClipType      string    `json:"clipType"`
	Status        string    `json:"status"`
	Progress      int       `json:"progress"`
	InputPath     string    `json:"inputPath"`
	OutputPath    string    `json:"outputPath,omitempty"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	AudioIndex    int       `json:"audioIndex"`
	CropBlackBars bool      `json:"cropBlackBars"`
}
