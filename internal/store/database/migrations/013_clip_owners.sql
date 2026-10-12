-- Each clip belongs to the Plex account that made it, by its plex.tv id. Clips
-- made before owners were recorded belong to the installation's owner, or to
-- nobody (0) when no account has claimed it.
ALTER TABLE clips ADD COLUMN owner_id INTEGER NOT NULL DEFAULT 0;
UPDATE clips SET owner_id = COALESCE((SELECT plex_user_id FROM users WHERE role = 'owner'), 0);
