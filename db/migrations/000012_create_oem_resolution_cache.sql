-- ============================================================================
-- Phase 2: Persistent OEM resolution cache (Postgres)
-- ============================================================================
-- Every successful OEM resolution (from TecDoc, dealer_lookup, partsouq,
-- prefix inference, sitemap, or any future federated source) writes to
-- this table. Second-visit queries return in <100 ms from Postgres — no
-- network, no MySQL slow-scan, no scraper 403.
--
-- Design principles:
--
--   * Never expires by default.
--       Positive facts about OEM parts don't age (a Kia Optima TF window
--       motor is still that in 2030). Callers may bypass this cache when
--       verifying a fact, but the cache itself never auto-invalidates.
--
--   * Corroboration boosts confidence.
--       When N sources agree on the same description for the same OEM,
--       corroborating_sources goes up. Confidence tracks accordingly:
--         1 source        -> 0.70
--         2 agreeing      -> 0.85
--         3+ agreeing     -> 0.95
--         verified_by_user -> 1.00 permanent
--
--   * Wrong results get downgraded, not silently kept.
--       oem_user_feedback (Phase 6) drives verified_by_user + downgrade.
-- ============================================================================

CREATE TABLE IF NOT EXISTS oem_resolution_cache (
    oem_normalized        TEXT PRIMARY KEY,        -- '824602T010' (no dashes, uppercase)
    oem_raw               TEXT NOT NULL,           -- '82460-2T010'
    description           TEXT NOT NULL,
    category              TEXT,
    make                  TEXT,
    model                 TEXT,
    year_start            INTEGER,
    year_end              INTEGER,
    confidence            DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    source                TEXT NOT NULL,           -- primary source that seeded this row
    source_url            TEXT,                    -- provenance
    corroborating_sources INTEGER NOT NULL DEFAULT 1 CHECK (corroborating_sources >= 1),
    corroborations        JSONB NOT NULL DEFAULT '[]'::jsonb,  -- [{"source": "tecdoc", "matched_at": "..."}, ...]
    verified_by_user      BOOLEAN NOT NULL DEFAULT FALSE,
    downgrade_count       INTEGER NOT NULL DEFAULT 0,          -- how many users flagged wrong
    first_seen_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_verified_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oem_cache_source
    ON oem_resolution_cache (source);

CREATE INDEX IF NOT EXISTS idx_oem_cache_confidence
    ON oem_resolution_cache (confidence DESC);

CREATE INDEX IF NOT EXISTS idx_oem_cache_make_model
    ON oem_resolution_cache (make, model)
    WHERE make IS NOT NULL;

-- Full-text search on description, for keyword-style lookups against the cache.
CREATE INDEX IF NOT EXISTS idx_oem_cache_desc_fts
    ON oem_resolution_cache USING gin (to_tsvector('english', COALESCE(description, '')));

COMMENT ON TABLE  oem_resolution_cache IS 'Phase 2 (2026-08-17): every successful OEM lookup lands here. Never auto-expires. Corroboration + user feedback drive confidence.';
COMMENT ON COLUMN oem_resolution_cache.corroborations IS 'JSONB array: [{"source":"tecdoc","matched_at":"2026-08-17T12:34:56Z"}, {"source":"dealer_kiapartsnow","matched_at":"..."}]';
