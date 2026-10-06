CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	data BYTEA NOT NULL,
	expires_at BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
