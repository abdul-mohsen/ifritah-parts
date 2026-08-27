-- ============================================================================
-- 000021 - aftermarket_online_cache (M8.T1)
-- ============================================================================
-- Persistent cache for online-source aftermarket lookups. All external
-- HTTP responses (eBay Motors API, schema.org JSON-LD from public reference
-- sites, category cross-reference sites) land here.
--
-- Cache-first read path: FindAftermarketForOEM_Online() checks this table
-- for fresh rows (fetched_at > NOW() - INTERVAL ttl_seconds) before falling
-- back to any outbound HTTP.
--
-- One row per (source, oem_normalized, brand, part_number) tuple.
-- Refreshed on cache miss.
--
-- Retention: nightly cron deletes rows older than 90 days regardless of
-- ttl_seconds to bound table size.
-- ============================================================================

CREATE TABLE IF NOT EXISTS aftermarket_online_cache (
    id              BIGSERIAL PRIMARY KEY,
    source          TEXT NOT NULL,                    -- 'ebay' / 'hyundaipartsdeal' / '7zap' / etc.
    oem_normalized  TEXT NOT NULL,                    -- LOWER(REPLACE(oem, '-', '')) — same normaliser as NormalizeOEM
    brand           TEXT,                             -- Post-NormalizeBrand canonical form
    part_number     TEXT NOT NULL,
    description     TEXT,
    price_cents     BIGINT DEFAULT 0,                 -- 0 = unknown
    currency        CHAR(3),
    condition       TEXT,                             -- 'new' / 'used' / 'reman' / 'oem_genuine' / 'unknown'
    image_url       TEXT,
    source_url      TEXT NOT NULL,                    -- click-through attribution URL
    raw_payload     JSONB,                            -- full source response for debugging
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ttl_seconds     INTEGER NOT NULL DEFAULT 2592000, -- 30 days default
    UNIQUE (source, oem_normalized, brand, part_number)
);

-- Primary read path: `WHERE oem_normalized = ? AND (fetched_at + ttl_seconds * INTERVAL '1 second') > NOW()`
CREATE INDEX IF NOT EXISTS idx_aftermarket_online_cache_oem
    ON aftermarket_online_cache (oem_normalized);

-- Freshness check: `WHERE source = ? AND fetched_at > NOW() - INTERVAL '30 days'`
CREATE INDEX IF NOT EXISTS idx_aftermarket_online_cache_source_freshness
    ON aftermarket_online_cache (source, fetched_at DESC);

-- Retention sweep: `DELETE WHERE fetched_at < NOW() - INTERVAL '90 days'`
-- Postgres requires index predicates to be IMMUTABLE (NOW() is not), so
-- the retention index is on fetched_at alone. Nightly cron uses it via a
-- range scan.
CREATE INDEX IF NOT EXISTS idx_aftermarket_online_cache_fetched_at
    ON aftermarket_online_cache (fetched_at);
