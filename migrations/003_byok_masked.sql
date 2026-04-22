-- WuNest migration 003 — BYOK UI polish.
--
-- Add a `masked` column to nest_byok so the list view can render a recognisable
-- preview ("sk-…6411") without decrypting every row on every page load. We
-- capture the mask at create-time when we still have plaintext; no backfill
-- needed for existing rows (there shouldn't be any yet in prod).

ALTER TABLE nest_byok ADD COLUMN IF NOT EXISTS masked TEXT NOT NULL DEFAULT '';
