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

// SaveToken stores the Plex access token for a client ID.
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//   - clientID: Plex X-Plex-Client-Identifier.
//   - accessToken: Typed string argument for SaveToken.
//
// Returns:
//   - err: Wrapped failure from "save token".
func (db *DB) SaveToken(ctx context.Context, clientID, accessToken string) error {
	_, err := db.conn.ExecContext(ctx, db.rewrite(`
		INSERT INTO plex_tokens (client_id, access_token, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(client_id) DO UPDATE SET
			access_token = excluded.access_token,
			updated_at = CURRENT_TIMESTAMP
	`), clientID, accessToken)
	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	return nil
}

// ClearAuth deletes stored Plex tokens and the selected server.
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//
// Returns:
//   - err: Wrapped failure such as "begin clear auth"; "clear tokens"; "clear
//     selected server".
func (db *DB) ClearAuth(ctx context.Context) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin clear auth: %w", err)
	}

	defer func() {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return
		}
	}()

	_, err = tx.ExecContext(ctx, db.rewrite(`DELETE FROM plex_tokens`))
	if err != nil {
		return fmt.Errorf("clear tokens: %w", err)
	}

	_, err = tx.ExecContext(ctx, db.rewrite(`DELETE FROM selected_server`))
	if err != nil {
		return fmt.Errorf("clear selected server: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit clear auth: %w", err)
	}

	return nil
}

// LatestToken returns the most recently stored Plex token.
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//
// Returns:
//   - token: The most recently stored Plex token.
//   - err: Wrapped failure from "latest token".
func (db *DB) LatestToken(ctx context.Context) (string, error) {
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

		return "", fmt.Errorf("latest token: %w", err)
	}

	return token, nil
}

// SaveSelectedServer upserts the single selected Plex server.
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//   - server: Plex Media Server connection (URL and token).
//
// Returns:
//   - err: Wrapped failure from "save selected server".
func (db *DB) SaveSelectedServer(ctx context.Context, server plex.Server) error {
	_, err := db.conn.ExecContext(ctx, db.rewrite(`
		INSERT INTO selected_server (id, name, address, port, scheme, token, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			address = excluded.address,
			port = excluded.port,
			scheme = excluded.scheme,
			token = excluded.token,
			updated_at = CURRENT_TIMESTAMP
	`), server.Name, server.Address, server.Port, server.Scheme, server.Token)
	if err != nil {
		return fmt.Errorf("save selected server: %w", err)
	}

	return nil
}

// SelectedServer loads the persisted Plex server, if any.
//
// Parameters:
//   - ctx: Cancels or deadlines this call.
//
// Returns:
//   - server: Selected or loaded Plex server.
//   - ok: True when the condition holds.
//   - err: Failure from selected server.
func (db *DB) SelectedServer(ctx context.Context) (plex.Server, bool, error) {
	var server plex.Server

	err := db.conn.QueryRowContext(
		ctx,
		db.rewrite(`SELECT name, address, port, scheme, token FROM selected_server WHERE id = 1`),
	).Scan(&server.Name, &server.Address, &server.Port, &server.Scheme, &server.Token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return server, false, nil
		}

		return server, false, fmt.Errorf("selected server: %w", err)
	}

	return server, true, nil
}
