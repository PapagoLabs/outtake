// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// LegacyToken returns the newest token in plex_tokens. It names the account
// that may claim an installation without an owner.
//
// Parameters:
//   - ctx: Request scope for the read.
//
// Returns:
//   - token: The newest stored token, or empty when none is stored.
//   - err: Non-nil when the read fails.
func (db *DB) LegacyToken(ctx context.Context) (string, error) {
	var token string

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(
			`SELECT access_token FROM plex_tokens ORDER BY updated_at DESC, id DESC LIMIT 1`,
		),
	).Scan(&token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}

		return "", fmt.Errorf("legacy token: %w", err)
	}

	return token, nil
}

// ClearLegacyTokens deletes every row in plex_tokens.
//
// Parameters:
//   - ctx: Request scope for the delete.
//
// Returns:
//   - err: Non-nil when the delete fails.
func (db *DB) ClearLegacyTokens(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, db.rewrite(`DELETE FROM plex_tokens`))
	if err != nil {
		return fmt.Errorf("clear legacy tokens: %w", err)
	}

	return nil
}

// SaveSelectedServer upserts the single selected Plex server.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - server: The Plex server the user selected.
//
// Returns:
//   - err: Non-nil when the row cannot be written.
func (db *DB) SaveSelectedServer(ctx context.Context, server plex.Server) error {
	_, err := db.conn.ExecContext(ctx, db.rewrite(`
		INSERT INTO selected_server (id, name, address, port, scheme, token, machine_id, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			address = excluded.address,
			port = excluded.port,
			scheme = excluded.scheme,
			token = excluded.token,
			machine_id = excluded.machine_id,
			updated_at = CURRENT_TIMESTAMP
	`), server.Name, server.Address, server.Port, server.Scheme, server.Token, server.MachineID)
	if err != nil {
		return fmt.Errorf("save selected server: %w", err)
	}

	return nil
}

// ClearSelectedServer forgets the persisted Plex server.
//
// Parameters:
//   - ctx: Request scope for the delete.
//
// Returns:
//   - err: Non-nil when the delete fails.
func (db *DB) ClearSelectedServer(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, db.rewrite(`DELETE FROM selected_server`))
	if err != nil {
		return fmt.Errorf("clear selected server: %w", err)
	}

	return nil
}

// SelectedServer loads the persisted Plex server, if any.
//
// Parameters:
//   - ctx: Request scope for the read.
//
// Returns:
//   - server: The stored server, or the zero value when none is stored.
//   - found: True when a server row was present.
//   - err: Non-nil when the read fails.
func (db *DB) SelectedServer(ctx context.Context) (plex.Server, bool, error) {
	var server plex.Server

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT name, address, port, scheme, token, machine_id
			FROM selected_server WHERE id = 1`),
	).Scan(&server.Name, &server.Address, &server.Port, &server.Scheme, &server.Token, &server.MachineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return server, false, nil
		}

		return server, false, fmt.Errorf("selected server: %w", err)
	}

	return server, true, nil
}
