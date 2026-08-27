-- ============================================================================
-- 000050 - search_feedback v2 (M6.S2.T1) — privacy-preserving hardening
-- ============================================================================
-- Evolves the v1 table created in 000016_search_feedback.sql:
--
--   * Adds an opaque `search_id` column so we can correlate one feedback
--     event with the search request that produced the result (frontend
--     mints a UUID at search time, passes it with every vote).
--   * Adds `result_article_id` (TecDoc legacyArticleId) so aggregations
--     can dedup by article, not just OEM string.
--   * Adds `result_part_num` so the report can label a downvoted result
--     ("Bosch F026 400 100 was disliked N times") without hitting TecDoc.
--   * Adds `user_hash` + `client_ip_hash` as SHA256 outputs — raw session
--     cookies and IP addresses NEVER touch the DB. Frontend sends the raw
--     values in a header/cookie; the backend hashes them BEFORE the row
--     is INSERTed. Enforced by the handler layer and by the tests.
--   * Adds `created_at` timestamp with sensible index ordering. The v1
--     column `submitted_at` is kept and back-filled so historical rows
--     stay queryable and the migration is idempotent.
--   * Widens the `verdict` CHECK constraint to accept `thumbs_up`,
--     `thumbs_down`, and `skip` alongside the legacy `up`/`down` values.
--     Legacy values are retained so back-fill / rollback stays possible.
--
-- Idempotency: every statement is CREATE ... IF NOT EXISTS or
-- ALTER TABLE ... ADD COLUMN IF NOT EXISTS or DROP CONSTRAINT IF EXISTS.
-- Safe to run on:
--   (a) a fresh DB where 000016 has never run (the CREATE TABLE stanza
--       below installs the full v2 schema in one shot);
--   (b) an existing DB where 000016 already installed v1 (each ALTER
--       adds the new columns and the CHECK constraint is replaced).
-- ============================================================================

-- (a) Fresh-DB path: install the complete v2 schema if the table doesn't
-- yet exist. On an upgrade path where 000016 already created the v1
-- table, this stanza is a no-op and the ALTER TABLE stanzas below take
-- over.
CREATE TABLE IF NOT EXISTS search_feedback (
    id                 BIGSERIAL PRIMARY KEY,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search_id          TEXT,
    query_oem          TEXT NOT NULL,
    result_article_id  INTEGER,
    result_brand       TEXT,
    result_part_num    TEXT,
    verdict            TEXT NOT NULL,
    reason             TEXT,
    user_hash          TEXT,
    client_ip_hash     TEXT,
    -- v1 columns kept for backward compat; new writes leave them NULL.
    result_oem         TEXT,
    result_desc        TEXT,
    session_id         TEXT,
    submitted_at       TIMESTAMPTZ,
    CONSTRAINT search_feedback_verdict_check
        CHECK (verdict IN ('thumbs_up', 'thumbs_down', 'skip', 'up', 'down'))
);

-- (b) Upgrade path: if 000016 already installed the table with the v1
-- schema, add the v2 columns. `ADD COLUMN IF NOT EXISTS` (Postgres ≥ 9.6)
-- is idempotent. Every new column is NULL-able so back-fill can happen
-- lazily and the migration never fails on legacy rows.
ALTER TABLE search_feedback ADD COLUMN IF NOT EXISTS created_at         TIMESTAMPTZ;
ALTER TABLE search_feedback ADD COLUMN IF NOT EXISTS search_id          TEXT;
ALTER TABLE search_feedback ADD COLUMN IF NOT EXISTS result_article_id  INTEGER;
ALTER TABLE search_feedback ADD COLUMN IF NOT EXISTS result_part_num    TEXT;
ALTER TABLE search_feedback ADD COLUMN IF NOT EXISTS user_hash          TEXT;
ALTER TABLE search_feedback ADD COLUMN IF NOT EXISTS client_ip_hash     TEXT;

-- Back-fill created_at from the v1 submitted_at column so aggregation
-- queries hitting `created_at` still see historical rows. `submitted_at`
-- was NOT NULL in v1, so this UPDATE is bounded.
UPDATE search_feedback
   SET created_at = submitted_at
 WHERE created_at IS NULL
   AND submitted_at IS NOT NULL;

-- Ensure created_at is present for every future row — cannot enforce
-- NOT NULL retroactively when the CREATE TABLE IF NOT EXISTS path was
-- not taken (rows already have NULL created_at? handled by the back-fill
-- above). Only set the default; leave the column NULL-able for safety.
ALTER TABLE search_feedback ALTER COLUMN created_at SET DEFAULT NOW();

-- Replace the v1 CHECK constraint (which only allowed 'up'/'down') with
-- the widened v2 version. DROP CONSTRAINT IF EXISTS is idempotent — on a
-- fresh DB the constraint was named `search_feedback_verdict_check` by
-- the CREATE TABLE stanza above, and on the upgrade path Postgres named
-- the inline CHECK from 000016 with a stable auto-generated name that
-- we also drop below.
ALTER TABLE search_feedback DROP CONSTRAINT IF EXISTS search_feedback_verdict_check;
ALTER TABLE search_feedback DROP CONSTRAINT IF EXISTS search_feedback_verdict_check1;
ALTER TABLE search_feedback ADD  CONSTRAINT search_feedback_verdict_check
    CHECK (verdict IN ('thumbs_up', 'thumbs_down', 'skip', 'up', 'down'));

-- Indexes — additive, IF NOT EXISTS.
CREATE INDEX IF NOT EXISTS idx_search_feedback_created_at
    ON search_feedback (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_search_feedback_query_oem
    ON search_feedback (query_oem);
CREATE INDEX IF NOT EXISTS idx_search_feedback_verdict_created_at
    ON search_feedback (verdict, created_at DESC);
