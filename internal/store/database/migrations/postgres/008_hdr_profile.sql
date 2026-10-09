UPDATE clip_profiles SET keep_hdr = 0 WHERE id = 'high';
INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width, is_default, keep_hdr)
SELECT 'high-hdr', 'High HDR', crf, preset, audio_kbps, max_width, 0, 1
FROM clip_profiles
WHERE id = 'high'
	AND NOT EXISTS (SELECT 1 FROM clip_profiles WHERE id = 'high-hdr' OR name = 'High HDR');
INSERT INTO clip_profiles (id, name, crf, preset, audio_kbps, max_width, is_default, keep_hdr)
SELECT 'high-hdr', 'High HDR', 18, 'slow', 320, 3840, 0, 1
WHERE NOT EXISTS (SELECT 1 FROM clip_profiles WHERE id = 'high-hdr' OR name = 'High HDR');
