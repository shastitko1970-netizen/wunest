-- M36 group-chat flow: per-swipe speaker attribution + per-chat flow config.
--
-- Two changes, both additive and safe to re-run:
--
-- 1. nest_messages.swipe_character_ids parallels the existing swipes[]
--    JSONB array. Index i in this array is the character_id that produced
--    swipes[i]. Required so the greeting flow (each character's first_mes
--    as a separate swipe) + future "this swipe was generated as Alice"
--    regenerate paths can keep the attribution straight. Nullable: when
--    missing, all swipes fall back to the message-level character_id
--    (same as pre-M36 behaviour).
--
-- 2. Mute / auto-speaker state lives in chat_metadata JSONB under the
--    `group` key (see ChatGroupMetadata in types.go) — no new column.
--    We store:
--       group.muted_character_ids  []UUID — excluded from speaker picker
--       group.auto_speaker         'manual' | 'round_robin'
--       group.last_speaker_id      UUID   — for round-robin pointer
--    JSONB is flexible so we can add fields later without a migration.

ALTER TABLE nest_messages
    ADD COLUMN IF NOT EXISTS swipe_character_ids UUID[] DEFAULT NULL;
