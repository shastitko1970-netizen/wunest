-- M37: full-text search across user's own chat messages.
--
-- Uses the `simple` text-search config (not 'russian' or 'english') on
-- purpose — WuNest is multilingual (RU + EN + mix + Cyrillic
-- transliteration). Stemming-based configs (russian/english) produce
-- language-specific normalisations that DEGRADE cross-language search:
-- e.g. Russian stemmer drops suffixes and English stemmer doesn't;
-- mixing them means typing "quests" in a Russian chat misses "quest".
-- 'simple' lowercases + splits on whitespace/punctuation but doesn't
-- stem — which gives us exact-word search that works across all
-- languages the user types in.
--
-- Generated column + GIN index = zero application code churn for
-- insert/update paths; Postgres maintains the index automatically.

ALTER TABLE nest_messages
    ADD COLUMN IF NOT EXISTS content_tsv tsvector
        GENERATED ALWAYS AS (to_tsvector('simple', coalesce(content, ''))) STORED;

CREATE INDEX IF NOT EXISTS idx_nest_messages_content_tsv
    ON nest_messages USING GIN(content_tsv);
