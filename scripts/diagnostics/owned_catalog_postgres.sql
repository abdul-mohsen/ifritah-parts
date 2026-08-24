-- ============================================================================
-- M0.T1 - owned_catalog diagnosis
-- ============================================================================
-- Purpose: figure out why `owned_catalog` mode returns 0 hits on qa for
-- every tested HK OEM. Two hypotheses to falsify:
--
--   H1: hk_parts_cache is empty (derive_worker never ran or ran but failed)
--   H2: hk_parts_cache is populated but the query in OwnedCatalogStrategy
--       uses a filter that eliminates all rows.
--
-- Run against qa Postgres (parts_engine database). Paste output into
-- docs/data-sources/owned-catalog-diagnosis.md.
-- ============================================================================

-- ── 1. Row count + last-updated timestamp ──────────────────────────────────
--    If total = 0: the derive worker hasn't populated the cache.
--    If total > 0 but max(created_at) is old: the worker ran once then stopped.

SELECT
  COUNT(*)                                     AS total_rows,
  COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '7 days')  AS rows_last_7d,
  COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '24 hours') AS rows_last_24h,
  MIN(created_at)                              AS oldest_row,
  MAX(created_at)                              AS newest_row
FROM hk_parts_cache;

-- ── 2. Distribution across prefix families (are ANY HK OEMs in there?) ─────

SELECT
  LEFT(oem_number, 2) AS prefix2,
  COUNT(*)            AS rows
FROM hk_parts_cache
GROUP BY prefix2
ORDER BY rows DESC
LIMIT 20;

-- ── 3. Direct lookup for the audit-corpus known-good OEMs ──────────────────
--    These MUST return rows for owned_catalog to work.

SELECT oem_number, brand, description, category, created_at
FROM hk_parts_cache
WHERE oem_number IN (
  '26350-2J001',
  '58101-3XA00',
  '97133-D3000',
  '82460-2T010',
  '27301-2E400',
  '26350-3C100',
  '58411-2SA00',
  '54630-2H000',
  '55311-2H000'
);

-- ── 4. Is there a normalised OEM column, and does it work? ─────────────────

SELECT column_name, data_type
FROM information_schema.columns
WHERE table_name = 'hk_parts_cache'
ORDER BY ordinal_position;

-- ── 5. Check the derive worker configuration state ─────────────────────────
--    (parts_derive_state may or may not exist, depending on migration 000011.)

SELECT * FROM parts_derive_state LIMIT 5;

-- ============================================================================
-- Expected results
-- ============================================================================
--
-- Healthy state:
--   Section 1: total_rows >= 100000; rows_last_7d >= 1000 (worker is refreshing)
--   Section 3: at least 6 of 9 corpus OEMs return rows
--
-- Broken (H1 - empty cache):
--   Section 1: total_rows = 0
--   Section 3: 0 rows returned
--
--   Fix: verify derive_worker.go is scheduled + wired to TecDoc MySQL on qa.
--        Check app logs for '[derive_worker]' lines. Manually trigger one
--        run and confirm rows land.
--
-- Broken (H2 - populated but query mismatch):
--   Section 1: total_rows > 0
--   Section 3: 0 rows returned
--
--   Fix: compare OwnedCatalogStrategy.Search() query to actual column
--        names + data (dash-form vs dashless in section 3).
