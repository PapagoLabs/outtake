-- A profile can copy the source's video when it already fits the profile, so
-- the clip needs no encode. The built-in Original profile does, keeps HDR, and
-- encodes like 4K HDR when a copy cannot be made. A name already in use is
-- kept, and that profile stands in for the built-in.
ALTER TABLE clip_profiles ADD COLUMN copy_video INTEGER NOT NULL DEFAULT 0;
INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width, is_default, keep_hdr, copy_video)
SELECT md5(random()::text || clock_timestamp()::text), 'Original', 18, 'slow', 256, 3840, 0, 1, 1
WHERE NOT EXISTS (SELECT 1 FROM clip_profiles WHERE name = 'Original');
