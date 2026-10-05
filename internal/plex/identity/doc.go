// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package identity owns who the caller is and which Plex server answers for
// them. It holds the Plex PIN login lifecycle, the process-wide server selection
// and the live sessions it polls, the session bookkeeping that carries the login
// result, and the CSRF token that protects the forms it submits.
package identity
