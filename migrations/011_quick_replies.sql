-- M39.2: quick-reply templates.
--
-- User-scoped list of short text snippets shown as chips above the
-- composer. Click → inserts the text into the draft; no auto-send
-- (so the user can edit before firing). ST convention — power-user
-- workflow tool for repeated phrases ("Continue please", "What
-- happens next?", "*looks around*").
--
-- Design: one flat list per user, ordered by `position`. No folders,
-- no sharing, no imports — all of those are scope creep for v1.
-- Revisit if users ask for preset packs.

CREATE TABLE IF NOT EXISTS nest_quick_replies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES nest_users(id) ON DELETE CASCADE,
    label TEXT NOT NULL,            -- short chip text
    text TEXT NOT NULL,              -- what actually gets inserted
    position INT NOT NULL DEFAULT 0, -- display order (lowest first)
    send_now BOOLEAN NOT NULL DEFAULT FALSE, -- true = auto-send on click
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_nest_quick_replies_user
    ON nest_quick_replies(user_id, position);
