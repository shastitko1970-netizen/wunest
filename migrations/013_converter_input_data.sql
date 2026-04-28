-- M51 Sprint 2 wave 2: persist converter input bytes for retry support.
--
-- Background:
--   - Original schema (012) stored only `input_sha256` + `input_size` —
--     the raw bytes were thrown away after the LLM call returned. So a
--     "retry with another model" button on a history row had no input
--     to re-feed: the user had to re-upload the file.
--   - Adding `input_data` BYTEA lets the new
--     POST /api/convert/{id}/retry endpoint reuse the original input
--     verbatim, both right after the conversion and after a page
--     refresh / cross-device sign-in.
--
-- Migration shape:
--   - NULL allowed so existing rows (created before deploy) don't blow
--     up. They simply can't be retried — the retry handler returns 410
--     "predates retry support" and asks the user to re-upload.
--   - Within 24h all such legacy rows are reaped by the existing
--     `nest-storage-reaper` cycle (see reaper.go), at which point all
--     remaining rows have input_data populated.
--
-- Storage budget:
--   500 KB max input × 3 jobs/hour cap × 24h retention × N users.
--   For 100 active users → at most ~360 MB. BYTEA in Postgres TOASTs
--   automatically when > ~2 KB, so the per-row overhead is small.

ALTER TABLE nest_converter_jobs
    ADD COLUMN IF NOT EXISTS input_data BYTEA NULL;

-- No index on input_data — it's only ever read by primary key in the
-- retry handler (Repository.GetWithInput). LIST queries deliberately
-- skip the column to keep page sizes small.
