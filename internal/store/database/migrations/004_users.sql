CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	plex_user_id INTEGER NOT NULL UNIQUE,
	username TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_login_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_single_owner ON users(role) WHERE role = 'owner';

CREATE TABLE IF NOT EXISTS app_settings (
	name TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
