-- WuNest migration 002 — World Info / Lorebook.
--
-- `nest_worlds` already exists from migration 001 (empty shell). This
-- migration rounds it out for M6:
--   * description column so users can label books without opening them
--   * user-id index (list-by-user is the primary query)
--   * flip default for `entries` from '{}' (object) to '[]' (array) since
--     our in-Go representation treats entries as an ordered list. Existing
--     rows with '{}' are coerced to '[]' so the Go decoder doesn't choke.
--
-- The M:M table `nest_character_worlds` attaches any number of lorebooks
-- to any character. V1 UI will expose "one primary attach" but the shape
-- supports multi-attach for free, which matches SillyTavern's model and
-- lets us layer a per-chat global book later without schema churn.

ALTER TABLE nest_worlds ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE nest_worlds ALTER COLUMN entries SET DEFAULT '[]'::jsonb;

-- Coerce legacy object-shaped entries to arrays so Go can decode them.
UPDATE nest_worlds SET entries = '[]'::jsonb WHERE jsonb_typeof(entries) <> 'array';

CREATE INDEX IF NOT EXISTS idx_nest_worlds_user ON nest_worlds(user_id);

CREATE TABLE IF NOT EXISTS nest_character_worlds (
    character_id UUID NOT NULL REFERENCES nest_characters(id) ON DELETE CASCADE,
    world_id     UUID NOT NULL REFERENCES nest_worlds(id)     ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (character_id, world_id)
);
CREATE INDEX IF NOT EXISTS idx_nest_character_worlds_world ON nest_character_worlds(world_id);
