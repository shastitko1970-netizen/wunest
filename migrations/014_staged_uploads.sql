-- Staged uploads: track MinIO objects uploaded before the user saves an entity.
-- The storage reaper keeps the latest active (unclaimed, unsuperseded) upload
-- per user+kind; older uploads in the same kind become eligible for deletion.

CREATE TABLE IF NOT EXISTS nest_staged_uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES nest_users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('avatar', 'background')),
    object_keys TEXT[] NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    claimed_at TIMESTAMPTZ,
    superseded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_nest_staged_uploads_user_kind_active
    ON nest_staged_uploads(user_id, kind)
    WHERE is_active AND claimed_at IS NULL AND superseded_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_nest_staged_uploads_keys
    ON nest_staged_uploads USING GIN(object_keys);
