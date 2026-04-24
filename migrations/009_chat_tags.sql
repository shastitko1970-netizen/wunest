-- M38.1: per-chat tags — flat array, not folders.
--
-- Design choice: flat tags over hierarchical folders. Tags scale better
-- to 100+ chats, have simpler UX ("type a word, hit enter") on mobile,
-- and don't force users to pick one category when a chat fits several.
-- Folders can always layer on top later as a virtual view over tag
-- combinations.
--
-- Tags are free-form text (user types whatever). No separate tag table
-- — dedup happens at read time via GIN + `&&` / `= ANY(...)` queries,
-- which keeps inserts cheap. Worst case a user accidentally creates
-- "RP" and "rp" as separate tags; we can normalise case in UI later if
-- that becomes a pain point.

ALTER TABLE nest_chats
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

-- GIN for "chats with any of these tags" (array overlap operator &&)
-- and "chats with all of these tags" (@> containment). Cheaper than a
-- separate tag-join table for the typical <5-tags-per-chat use case.
CREATE INDEX IF NOT EXISTS idx_nest_chats_tags
    ON nest_chats USING GIN(tags);
