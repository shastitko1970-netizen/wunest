-- M43: theme converter jobs.
--
-- /convert route lets a user upload a SillyTavern theme .json, run it
-- through an LLM (their BYOK key or WuApi pool), and receive a WuNest-
-- compatible .json back. Each conversion is saved for 24h so the user
-- can share the resulting link with friends / copy later; after that
-- the row is purged by a background reaper.
--
-- Design:
--   - One row per conversion. `output_json` populated when done.
--   - `status` tracks pending/running/done/error — last two are terminal.
--   - `expires_at` = created_at + 24h; reaper deletes rows past expiry.
--   - Rate limiter uses `created_at DESC` index to count recent rows.
--   - `byok_id` null → WuApi pool was used; non-null → that BYOK key.
--
-- Size bounds enforced at handler level (500KB input max); JSONB stores
-- the resulting theme so GETs don't need a second roundtrip.

CREATE TABLE IF NOT EXISTS nest_converter_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES nest_users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',   -- pending | running | done | error
    model TEXT NOT NULL,                       -- e.g. "anthropic/claude-sonnet-4-5"
    byok_id UUID NULL,                         -- nullable: null = WuApi pool
    input_sha256 TEXT NOT NULL,                -- hex sha256 of input JSON for dedup
    input_size INT NOT NULL,                   -- bytes of input JSON
    output_json JSONB NULL,                    -- final WuNest theme on success
    error_message TEXT NULL,                   -- populated on status=error
    tokens_in INT NOT NULL DEFAULT 0,
    tokens_out INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '24 hours'),
    finished_at TIMESTAMPTZ NULL
);

-- Rate-limit lookup: "how many jobs in the last hour for this user?"
CREATE INDEX IF NOT EXISTS idx_nest_converter_jobs_user_created
    ON nest_converter_jobs(user_id, created_at DESC);

-- Reaper lookup: find expired rows fast.
CREATE INDEX IF NOT EXISTS idx_nest_converter_jobs_expires
    ON nest_converter_jobs(expires_at);
