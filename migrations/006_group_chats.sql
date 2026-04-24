-- M35 group-chat support.
--
-- One chat can now reference multiple characters. Single-character chats
-- (the only kind pre-M35) continue to work via the existing
-- nest_chats.character_id column — we leave that untouched so every
-- query that filters by it keeps returning the same rows. The new
-- character_ids array is the source of truth going forward; repo
-- helpers keep `character_id` in sync with the first element for
-- backward compatibility.
--
-- On assistant messages we record WHICH character produced the text
-- (nest_messages.character_id) so the UI can attribute each message
-- to its speaker. user/system messages leave this null.

ALTER TABLE nest_chats
    ADD COLUMN IF NOT EXISTS character_ids UUID[] NOT NULL DEFAULT '{}';

-- Backfill: any existing single-character chat becomes a 1-element array.
-- Running twice is safe — WHERE excludes already-filled rows.
UPDATE nest_chats
   SET character_ids = ARRAY[character_id]
 WHERE character_id IS NOT NULL
   AND (character_ids IS NULL OR cardinality(character_ids) = 0);

-- Per-message speaker attribution (assistant messages in group chats).
-- Nullable — user/system messages + legacy single-char assistant
-- messages leave it null and the UI falls back to chat.character_id.
ALTER TABLE nest_messages
    ADD COLUMN IF NOT EXISTS character_id UUID REFERENCES nest_characters(id) ON DELETE SET NULL;

-- Composite index for "all messages by this character in this chat"
-- — useful for future features like per-character memory/summary.
CREATE INDEX IF NOT EXISTS idx_nest_messages_chat_character
    ON nest_messages(chat_id, character_id)
    WHERE character_id IS NOT NULL;
