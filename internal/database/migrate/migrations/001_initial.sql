CREATE TABLE IF NOT EXISTS plex_tokens (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	client_id TEXT NOT NULL UNIQUE,
	access_token TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS clips (
	id TEXT PRIMARY KEY,
	media_id TEXT NOT NULL,
	media_title TEXT NOT NULL,
	media_type TEXT NOT NULL DEFAULT 'movie',
	clip_type TEXT NOT NULL DEFAULT 'clip',
	status TEXT NOT NULL DEFAULT 'pending',
	progress INTEGER NOT NULL DEFAULT 0,
	input_path TEXT NOT NULL,
	output_path TEXT,
	start_time REAL NOT NULL,
	duration REAL NOT NULL,
	quality TEXT NOT NULL DEFAULT 'medium',
	width INTEGER NOT NULL DEFAULT 0,
	fps INTEGER NOT NULL DEFAULT 0,
	error_message TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	name TEXT NOT NULL DEFAULT '',
	audio_index INTEGER NOT NULL DEFAULT 0,
	crop_black_bars INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS selected_server (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	name TEXT NOT NULL,
	address TEXT NOT NULL,
	port INTEGER NOT NULL,
	scheme TEXT NOT NULL,
	token TEXT NOT NULL,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS clip_profiles (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	crf INTEGER NOT NULL,
	preset TEXT NOT NULL,
	audio_kbps INTEGER NOT NULL DEFAULT 192,
	max_width INTEGER NOT NULL DEFAULT 1920,
	is_default INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width, is_default) VALUES
	('low', 'Low', 28, 'veryfast', 128, 1280, 0),
	('medium', 'Medium', 23, 'medium', 192, 1920, 1),
	('high', 'High', 18, 'slow', 320, 3840, 0);

CREATE INDEX IF NOT EXISTS idx_clips_status ON clips(status);
CREATE INDEX IF NOT EXISTS idx_clips_media_id ON clips(media_id);
