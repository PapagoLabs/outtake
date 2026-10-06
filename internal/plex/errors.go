// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"errors"
)

var (
	// ErrUnauthorized is returned when Plex rejects the token.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrPlexError is returned when Plex responds with a failure status.
	ErrPlexError = errors.New("plex error")

	// ErrServerReturnedError is returned when a Plex server request fails.
	ErrServerReturnedError = errors.New("server returned error")

	// ErrNoFilePathFound is returned when media metadata has no file path.
	ErrNoFilePathFound = errors.New("no file path found")

	// ErrPINNotYetClaimed is returned when a PIN has not been authorized.
	ErrPINNotYetClaimed = errors.New("PIN not yet claimed")

	// ErrInvalidThumbPath is returned when a thumbnail path is not a library asset.
	ErrInvalidThumbPath = errors.New("invalid thumbnail path")

	// ErrNotImage is returned when a thumbnail response is not an image.
	ErrNotImage = errors.New("thumbnail is not an image")

	// ErrInvalidMediaID is returned when a media id is not a Plex rating key.
	ErrInvalidMediaID = errors.New("invalid media id")

	// ErrUnsupportedSectionIndex is returned when a section facet is not firstCharacter or year.
	ErrUnsupportedSectionIndex = errors.New("unsupported section index")

	// errEmptyBody is the error for a Plex response body that is empty.
	errEmptyBody = errors.New("decode: empty body")
)
