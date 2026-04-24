-- M38.4: chat memory / auto-summarisation.
--
-- When a chat's history grows to the point where we're about to hit
-- the model's context window, we ask a cheap LLM to summarise the
-- oldest N messages into a single narrative paragraph. That summary
-- replaces those messages in the outbound prompt while the original
-- rows stay in nest_messages for display. Keeps the scene's memory
-- usable 200+ turns in.
--
-- Design:
--   - One "rolling" summary per chat that covers messages up to a
--     certain point. Regenerate it by appending more messages and
--     replacing in-place (no summary history — reduces storage +
--     confusion about "which version is current"). User can still
--     edit content manually; server tracks covered_through_message_id
--     to know what's included.
--   - Additional "manual" summaries can be added by the user (e.g.
--     "here's what happened in Act 1"). Those are free-form + not
--     auto-replaced.
--   - `role` field distinguishes auto vs manual vs user-pinned.

CREATE TABLE IF NOT EXISTS nest_chat_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id UUID NOT NULL REFERENCES nest_chats(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'auto', -- auto | manual | pinned
    covered_through_message_id BIGINT REFERENCES nest_messages(id) ON DELETE SET NULL,
    token_count INT NOT NULL DEFAULT 0,
    model TEXT NOT NULL DEFAULT '', -- which model generated it (for audit)
    position INT NOT NULL DEFAULT 0, -- display order in the memory drawer
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_nest_chat_summaries_chat
    ON nest_chat_summaries(chat_id, position ASC);

-- Fast "does this chat have an auto-summary yet?" lookup.
CREATE INDEX IF NOT EXISTS idx_nest_chat_summaries_chat_role
    ON nest_chat_summaries(chat_id) WHERE role = 'auto';
