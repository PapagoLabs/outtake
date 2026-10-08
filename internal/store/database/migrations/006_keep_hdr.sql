UPDATE clips SET preserve_hdr = CASE WHEN web_safe_color = 0 THEN 1 ELSE 0 END;
ALTER TABLE clip_profiles ADD COLUMN keep_hdr INTEGER NOT NULL DEFAULT 0;
UPDATE clip_profiles SET keep_hdr = 1 WHERE id = 'high';
