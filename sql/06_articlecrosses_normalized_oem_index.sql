-- ============================================================================
-- MySQL migration 06 — indexed OEM lookup for TecDoc queries
-- ============================================================================
-- Fixes two P0 performance issues found in the 2026-08-19 data-quality audit
-- (docs/reports/2026-08-19-post-pr14-data-quality.md §5, §7.1):
--
-- (A) articlecrosses.SearchCrossReferences — 3-8 HOUR full-table scans
-- (B) oem_number.SearchByOEM.primary       — 2-3 SECOND per query, flagged
--                                            SQL SLOW on every request
--
-- Root causes:
--
--   (A) PR #14 needed `WHERE LOWER(REPLACE(REPLACE(REPLACE(REPLACE(
--       ac.oemNumber, '-', ''), ' ', ''), '.', ''), '/', ''))` because the
--       TecDoc 2020 schema has NO pre-normalized column on the
--       articlecrosses table. The function-on-column wrapping disables the
--       index on ac.oemNumber → full scan of 30M rows.
--
--       Debug-log evidence from qa.ifritah.com:
--
--         SQL SLOW ⚠⚠  TecDocCrossRef.SearchCrossReferences: 3h6m27s   — SELECT ac.oe...
--         SQL SLOW ⚠⚠  TecDocCrossRef.SearchCrossReferences: 5h36m58s  — SELECT ac.o...
--         SQL SLOW ⚠⚠  TecDocCrossRef.SearchCrossReferences: 8h10m31s  — SELECT ac.o...
--
--   (B) oem_number.clean_number IS a plain equality WHERE clause
--       (WHERE on2.clean_number = ?) — so a BTREE index would work — but the
--       deployed schema only has a FULLTEXT index on the number column.
--       FULLTEXT does NOT accelerate equality lookups → 21.5M row scan.
--
-- ═══════════════════════════════════════════════════════════════════════════
-- Fixes in this migration
-- ═══════════════════════════════════════════════════════════════════════════
--
--   (A) Add STORED GENERATED COLUMN oemNumberNormalized on articlecrosses,
--       computed at insert/update time using the exact same normalization
--       the caller performs (LOWER + strip -, space, dot, slash), plus a
--       BTREE index on it. Query becomes WHERE ac.oemNumberNormalized = ?
--       → O(log n) index seek, sub-10 ms per lookup.
--
--   (B) Add a BTREE index on oem_number.clean_number. The column exists;
--       just needs an index of the right type for equality lookups.
--       Query becomes O(log n) instead of O(n). Expected latency: 2-3 s → <5 ms.
--
-- Both changes:
--   * Preserve original data — generated columns are DERIVED, new indexes
--     don't mutate rows.
--   * Are ONLINE on MySQL 8.0+; on MySQL 5.7 they block writes for the DDL
--     duration (~5-15 min on the 30M-row articlecrosses, ~2-5 min on the
--     21.5M-row oem_number).
--   * Are individually reversible via DROP INDEX / DROP COLUMN.
--
-- ROLLBACK:
--
--   ALTER TABLE oem_number DROP INDEX idx_oem_number_clean_number;
--   ALTER TABLE articlecrosses DROP INDEX idx_articlecrosses_oemNumberNormalized;
--   ALTER TABLE articlecrosses DROP COLUMN oemNumberNormalized;
--
-- COMPANION Go CHANGE:
--
--   internal/service/tecdoc_crossref.go — queries use
--   WHERE ac.oemNumberNormalized = ? (indexed generated column).
--
--   internal/service/tecdoc.go — no code change needed; the existing query
--   WHERE on2.clean_number = ? starts using the new index automatically.
-- ============================================================================

-- ── (A.1) Add the generated column on articlecrosses ───────────────────────

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

-- ── (A.2) Add the BTREE index on the generated column ──────────────────────
--
-- Index name kept explicit (`idx_articlecrosses_oemNumberNormalized`) so it's
-- easy to correlate with query plans and rollback scripts.

CREATE INDEX idx_articlecrosses_oemNumberNormalized
  ON articlecrosses (oemNumberNormalized);

-- ── (B) Add BTREE index on oem_number.clean_number ─────────────────────────
--
-- The deployed schema only has a FULLTEXT index on oem_number.number, which
-- doesn't help the WHERE on2.clean_number = ? equality lookup that
-- TecDoc.SearchByOEM.primary runs on every OEM search. This BTREE index
-- turns that from a 21.5M-row scan (2-3 s per query, flagged SQL SLOW every
-- time in the qa debug log) into a millisecond index seek.

CREATE INDEX idx_oem_number_clean_number
  ON oem_number (clean_number);

-- ── Verification queries (advisory, no side effects) ───────────────────────
-- Uncomment and run these AFTER the DDL completes to confirm the new
-- columns + indexes are healthy.
--
-- -- Confirm generated column populated on every row:
-- SELECT COUNT(*) AS total_rows,
--        COUNT(oemNumberNormalized) AS populated_rows,
--        COUNT(*) - COUNT(oemNumberNormalized) AS null_rows
-- FROM articlecrosses;
--
-- -- Confirm indexes are used by the query planner (should show "ref" access
-- --   type on the new indexes):
-- EXPLAIN
--   SELECT ac.oemNumber, ac.legacyArticleId
--   FROM articlecrosses ac
--   WHERE ac.oemNumberNormalized = '824602t010' LIMIT 20;
--
-- EXPLAIN
--   SELECT on2.number, on2.articleId
--   FROM oem_number on2
--   WHERE on2.clean_number = '824602t010' LIMIT 20;
--
-- -- Sanity check: known-good OEM 82460-2T010 returns rows in <10 ms:
-- SELECT COUNT(*) FROM articlecrosses WHERE oemNumberNormalized = '824602t010';
-- SELECT COUNT(*) FROM oem_number WHERE clean_number = '824602t010';
