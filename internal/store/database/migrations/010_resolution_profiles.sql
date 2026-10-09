-- The built-in clip profiles are named after what they produce and take
-- generated ids like every other profile. A name already in use is kept, and
-- that profile stands in for the built-in of the same name.
INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width, is_default, keep_hdr)
SELECT lower(hex(randomblob(16))), '720p', 21, 'medium', 160, 1280, 0, 0
WHERE NOT EXISTS (SELECT 1 FROM clip_profiles WHERE name = '720p');
INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width, is_default, keep_hdr)
SELECT lower(hex(randomblob(16))), '1080p', 20, 'medium', 192, 1920, 0, 0
WHERE NOT EXISTS (SELECT 1 FROM clip_profiles WHERE name = '1080p');
INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width, is_default, keep_hdr)
SELECT lower(hex(randomblob(16))), '4K', 18, 'slow', 256, 3840, 0, 0
WHERE NOT EXISTS (SELECT 1 FROM clip_profiles WHERE name = '4K');
INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width, is_default, keep_hdr)
SELECT lower(hex(randomblob(16))), '4K HDR', 18, 'slow', 256, 3840, 0, 1
WHERE NOT EXISTS (SELECT 1 FROM clip_profiles WHERE name = '4K HDR');

-- Clips move from each old built-in, by its id, to the profile that replaces it.
UPDATE clips SET quality = (SELECT id FROM clip_profiles WHERE name = '720p') WHERE quality = 'low';
UPDATE clips SET quality = (SELECT id FROM clip_profiles WHERE name = '1080p') WHERE quality = 'medium';
UPDATE clips SET quality = (SELECT id FROM clip_profiles WHERE name = '4K') WHERE quality = 'high';
UPDATE clips SET quality = (SELECT id FROM clip_profiles WHERE name = '4K HDR') WHERE quality = 'high-hdr';

-- An old built-in renamed to one of the new names is the profile its clips
-- just moved to, so it stays and keeps its id.
DELETE FROM clip_profiles
WHERE id IN ('low', 'medium', 'high', 'high-hdr')
	AND name NOT IN ('720p', '1080p', '4K', '4K HDR');

-- A default chosen among the custom profiles stays. One that was a built-in
-- passes to 1080p.
UPDATE clip_profiles SET is_default = 1
WHERE name = '1080p'
	AND NOT EXISTS (SELECT 1 FROM clip_profiles WHERE is_default = 1);
