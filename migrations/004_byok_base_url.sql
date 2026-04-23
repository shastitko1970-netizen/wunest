-- M24 BYOK enhancement: store the provider's API root alongside the key.
-- Without this, WuNest has to guess the endpoint per-request; with it,
-- each key carries its own routing target so a chat pinned to a BYOK can
-- go direct to the provider (bypassing WuApi's proxy) with a known URL.
--
-- Existing rows get an empty string; a follow-up app-side backfill in
-- Repo.List will substitute the canonical URL per-provider at read time
-- so users who created keys before this column existed don't have to
-- re-enter them.
ALTER TABLE nest_byok
  ADD COLUMN IF NOT EXISTS base_url TEXT NOT NULL DEFAULT '';
