-- A clip its owner shared is visible to the installation's other users who can
-- see its source title in Plex. Every clip starts private.
ALTER TABLE clips ADD COLUMN shared INTEGER NOT NULL DEFAULT 0;
