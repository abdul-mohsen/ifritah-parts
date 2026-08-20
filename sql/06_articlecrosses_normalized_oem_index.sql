-- ============================================================================
-- MySQL migration 06 — articlecrosses.oemNumberNormalized generated column + index
-- ============================================================================
-- FIXES a P0 performance issue found in the 2026-08-19 data-quality audit
-- (docs/reports/2026-08-19-post-pr14-data-quality.md §5, §7.1):
--
--   PR #14 changed `TecDocCrossRef.SearchCrossReferences` WHERE clause to
--
--     WHERE LOWER(REPLACE(REPLACE(REPLACE(REPLACE(ac.oemNumber, '-', ''),
--                                                 ' ', ''), '.', ''), '/', '')) = ?
--
--   because the deployed TecDoc 2020 schema does not have `cleanCrossNumber`
--   or `articleCrossNumber` columns. This SQL is correct — but the
--   `LOWER(REPLACE(...))` wrapping the column disables the index on
--   `ac.oemNumber`, forcing a full table scan of the 30M-row
--   `articlecrosses` table on every request.
--
--   Debug-log evidence from qa.ifritah.com after PR #14 deploy:
--
--     SQL SLOW ⚠⚠  TecDocCrossRef.SearchCrossReferences: 3h6m27s   — SELECT ac.oe...
--     SQL SLOW ⚠⚠  TecDocCrossRef.SearchCrossReferences: 5h36m58s  — SELECT ac.o...
--     SQL SLOW ⚠⚠  TecDocCrossRef.SearchCrossReferences: 8h10m31s  — SELECT ac.o...
--     SQL SLOW ⚠⚠  TecDocCrossRef.SearchCrossReferences: 7h24m11s  — SELECT ac.o...
--
--   The 15-second Go-side context deadline fires long before the query
--   completes, but MySQL keeps churning through the full scan for hours.
--
-- FIX in this migration:
--
--   Add a STORED GENERATED COLUMN `oemNumberNormalized` — computed once at
--   insert/update time from `oemNumber`, using the exact same normalization
--   the caller performs (LOWER + strip -, space, dot, slash). Then create
--   an index on the generated column.
--
--   The Go query then becomes:
--
--     WHERE ac.oemNumberNormalized = ?
--
--   which uses the new index → O(log n) instead of O(n). Expected latency
--   drops from 3-8 HOURS to <10 ms per lookup.
--
-- SAFETY:
--
--   * Generated columns are DERIVED — they never mutate the source
--     `oemNumber` column. Existing readers that never look at the new
--     column are unaffected.
--   * STORED (not VIRTUAL) so the index is fully materialised on disk;
--     query cost is a plain index lookup.
--   * The ADD COLUMN + ADD INDEX is a single online DDL in MySQL 8.0+;
--     on MySQL 5.7 it will block writes (still fast — one-time cost).
--   * On a 30M-row table, expect ~5-15 minutes of DDL time and ~500-1500 MB
--     of index storage. Monitor `SHOW ENGINE INNODB STATUS` if in doubt.
--
-- ROLLBACK:
--
--   ALTER TABLE articlecrosses DROP INDEX idx_articlecrosses_oemNumberNormalized;
--   ALTER TABLE articlecrosses DROP COLUMN oemNumberNormalized;
--
--   Both operations are online on MySQL 8.0+ and preserve original data.
--
-- COMPANION Go CHANGE:
--
--   internal/service/tecdoc_crossref.go — QueryCrossRefs + QueryCrossRefsBatch
--   probe `information_schema.columns` at startup. If the generated column
--   exists, they use the fast query. If not (fresh dump without this
--   migration applied yet), they fall back to the slow query with a WARN log
--   so ops notices the drift.
-- ============================================================================

-- ── Step 1: Add the generated column ───────────────────────────────────────
--
-- MySQL 8.0+ supports STORED generated columns natively. On MySQL 5.7 the
-- ADD COLUMN + ADD INDEX below still works but blocks writes for the DDL
-- duration.

ALTER TABLE articlecrosses
  ADD COLUMN oemNumberNormalized VARCHAR(50)
    GENERATED ALWAYS AS (
      LOWER(
        REPLACE(
          REPLACE(
            REPLACE(
              REPLACE(oemNumber, '-', ''),
              ' ', ''
            ),
            '.', ''
          ),
          '/', ''
        )
      )
    ) STORED;

-- ── Step 2: Add the index ──────────────────────────────────────────────────
--
-- Index name kept explicit (`idx_articlecrosses_oemNumberNormalized`) so the
-- Go-side column-detection probe can key on it. Any future rename must be
-- reflected in internal/service/tecdoc_crossref.go probeGeneratedColumn().

CREATE INDEX idx_articlecrosses_oemNumberNormalized
  ON articlecrosses (oemNumberNormalized);

-- ── Step 3: Verification queries (advisory, no side effects) ───────────────
--
-- Run these AFTER the DDL completes to confirm the new column + index are
-- healthy and returning correct rows. Uncomment for manual verification.
--
-- -- Confirm column exists and is populated on every row:
-- SELECT COUNT(*) AS total_rows,
--        COUNT(oemNumberNormalized) AS populated_rows,
--        COUNT(*) - COUNT(oemNumberNormalized) AS null_rows
-- FROM articlecrosses;
--
-- -- Confirm index is used by the planner (should show "index" access on
-- --   idx_articlecrosses_oemNumberNormalized):
-- EXPLAIN
--   SELECT ac.oemNumber,
--          COALESCE(m.manuName, ''),
--          COALESCE(a.legacyArticleId, 0),
--          COALESCE(a.articleNumber, ''),
--          COALESCE(a.genericArticleDescription, ''),
--          COALESCE(ac.brandName, ''),
--          COALESCE(m.manuName, '')
--   FROM articlecrosses ac
--   LEFT JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
--   LEFT JOIN manufacturers m ON m.manuId = ac.mfrId AND m.linkingTargetType = 'P'
--   WHERE ac.oemNumberNormalized = '824602t010'
--   LIMIT 20;
--
-- -- Sanity check: does the known-good OEM 82460-2T010 now return rows in <10 ms?
-- --   (Should show similar rows to those returned by the pre-migration
-- --   `LOWER(REPLACE(...)) = ?` query, but in milliseconds not hours.)
-- SELECT COUNT(*)
-- FROM articlecrosses
-- WHERE oemNumberNormalized = '824602t010';
