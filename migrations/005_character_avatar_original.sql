-- M33 object storage: store the original-size avatar URL alongside the
-- thumbnail URL currently held in `avatar_url`.
--
-- On import, the character's PNG is uploaded to MinIO twice: once as-is
-- (original) and once resized to a 400×400 thumbnail. The thumbnail URL
-- stays in `avatar_url` so existing reads keep working; the full-size
-- URL goes here for detail views.
--
-- Nullable — pre-M33 characters have no original stored (the source card
-- was never persisted), and new characters may ship without an avatar at
-- all (e.g. created through "New character" rather than PNG import).
ALTER TABLE nest_characters
  ADD COLUMN IF NOT EXISTS avatar_original_url TEXT;
