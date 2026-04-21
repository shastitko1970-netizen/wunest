-- WuNest initial schema.
--
-- All tables are prefixed `nest_` to make the namespace obvious if this DB
-- is ever consolidated with WuApi's schema. Keep the prefix even though we
-- deploy into a dedicated `wunest` database — it's cheap insurance.

-- Required extensions (Postgres 13+).
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ─── nest_users ─────────────────────────────────────────────────
-- Shadow copy of a WuApi user. Created on first WuNest login.
-- WuApi user_id is kept as a plain int — no FK, WuApi lives in another DB.
CREATE TABLE IF NOT EXISTS nest_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wuapi_user_id BIGINT NOT NULL UNIQUE,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── nest_characters ────────────────────────────────────────────
-- Character cards (V2/V3 compatible JSON payload in `data`).
CREATE TABLE IF NOT EXISTS nest_characters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES nest_users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    data JSONB NOT NULL,
    avatar_url TEXT,
    tags TEXT[] NOT NULL DEFAULT '{}',
    favorite BOOLEAN NOT NULL DEFAULT FALSE,
    spec TEXT NOT NULL DEFAULT 'chara_card_v3',
    source_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nest_characters_user ON nest_characters(user_id);
CREATE INDEX IF NOT EXISTS idx_nest_characters_tags ON nest_characters USING GIN(tags);

-- ─── nest_chats ─────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS nest_chats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES nest_users(id) ON DELETE CASCADE,
    character_id UUID REFERENCES nest_characters(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    chat_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nest_chats_user_char ON nest_chats(user_id, character_id);

-- ─── nest_messages ──────────────────────────────────────────────
-- Single row per message. Swipes (alternative assistant outputs) live in JSON.
CREATE TABLE IF NOT EXISTS nest_messages (
    id BIGSERIAL PRIMARY KEY,
    chat_id UUID NOT NULL REFERENCES nest_chats(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content TEXT NOT NULL,
    swipes JSONB NOT NULL DEFAULT '[]'::jsonb,
    swipe_id INTEGER NOT NULL DEFAULT 0,
    extras JSONB NOT NULL DEFAULT '{}'::jsonb,
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nest_messages_chat ON nest_messages(chat_id, id);

-- ─── nest_personas ──────────────────────────────────────────────
-- User-facing personas (the "you" the character responds to).
CREATE TABLE IF NOT EXISTS nest_personas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES nest_users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    avatar_url TEXT,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nest_personas_user ON nest_personas(user_id);

-- ─── nest_worlds (Lorebook / World Info) ────────────────────────
CREATE TABLE IF NOT EXISTS nest_worlds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES nest_users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    entries JSONB NOT NULL DEFAULT '{}'::jsonb,
    extensions JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── nest_presets ───────────────────────────────────────────────
-- Unified bucket for sampler / instruct / context / sysprompt / reasoning presets.
CREATE TABLE IF NOT EXISTS nest_presets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES nest_users(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('sampler', 'openai', 'instruct', 'context', 'sysprompt', 'reasoning')),
    name TEXT NOT NULL,
    data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, type, name)
);

-- ─── nest_byok ──────────────────────────────────────────────────
-- Bring-your-own-key: optional user-supplied provider keys.
-- Encrypted with AES-GCM using SECRETS_KEY env var.
CREATE TABLE IF NOT EXISTS nest_byok (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES nest_users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    key_encrypted BYTEA NOT NULL,
    key_nonce BYTEA NOT NULL,
    label TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, provider, label)
);
