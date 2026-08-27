-- ============================================================================
-- TecDoc Baseline Regression Check
-- ============================================================================
-- The full audit ran to completion on 2026-08-27 05:32:52. Complete
-- findings are pinned locally at (not in the repo):
--
--   C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\tecdoc-known-answers\ANSWERED.md
--
-- This file is what's left after crossing off every answered query. It's
-- the minimum diagnostic worth re-running whenever:
--   * You apply a new sql/XX migration
--   * You refresh the TecDoc dump
--   * FindBySpecMatch performance regresses in production
--   * You suspect an index went missing
--
-- 2 sections, ~2 seconds to run. Every query is metadata-only or
-- indexed-single-row so it's safe to run against a prod replica any
-- time.
--
-- Usage:
--
--   mysql --host=<tecdoc-mysql-host> --user=<user> --password \
--         --database=<tecdoc-db-name> \
--         < scripts/diagnostics/tecdoc_diagnostic_full.sql \
--         > tecdoc-baseline-$(date +%Y-%m-%d).txt
--
-- Everything else (data coverage, corpus verification, aftermarket brand
-- probe) is one-time discovery — the answers don't change unless the
-- TecDoc dump changes. Full 40-KB historical version lives in git
-- history at `4646b3a:scripts/diagnostics/tecdoc_diagnostic_full.sql`.
-- ============================================================================


SELECT '' AS ' ';
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '  TECDOC BASELINE REGRESSION CHECK' AS ' ';
SELECT '  Run at:' AS ' ', NOW() AS run_at;
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '' AS ' ';


-- ─── §A P0 index PASS/FAIL ─────────────────────────────────────────
--
-- All 5 should say PRESENT (confirmed 2026-08-27). Any MISSING here
-- means a migration regressed — check the corresponding sql/06 or
-- sql/07 or sql/08_articlecriteria_criteria_value_hotfix.sql migration.

SELECT '─── §A P0 index PASS/FAIL ────────────────────────────' AS section;

SELECT
	'articlecrosses.oemNumberNormalized (column)' AS check_name,
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'articlecrosses'
		  AND column_name = 'oemNumberNormalized'
	) THEN 'PRESENT' ELSE 'MISSING (sql/06 needed)' END AS status
UNION ALL SELECT
	'idx_articlecrosses_oemNumberNormalized',
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'articlecrosses'
		  AND index_name = 'idx_articlecrosses_oemNumberNormalized'
	) THEN 'PRESENT' ELSE 'MISSING (sql/06 needed)' END
UNION ALL SELECT
	'idx_oem_number_clean_number',
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'oem_number'
		  AND index_name = 'idx_oem_number_clean_number'
	) THEN 'PRESENT' ELSE 'MISSING (sql/06 needed)' END
UNION ALL SELECT
	'idx_articlecriteria_legacyArticleId',
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'articlecriteria'
		  AND index_name = 'idx_articlecriteria_legacyArticleId'
	) THEN 'PRESENT' ELSE 'MISSING (sql/07 needed)' END
UNION ALL SELECT
	'idx_articlecriteria_criteria_value',
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'articlecriteria'
		  AND index_name = 'idx_articlecriteria_criteria_value'
	) THEN 'PRESENT' ELSE 'MISSING (sql/08 hotfix needed)' END;


-- ─── §F EXPLAIN — every hot production query hits an index ─────────
--
-- 2026-08-27 baseline: every query below returned type=ref against the
-- correct index. If any drops back to type=ALL (full scan), the
-- corresponding index has been dropped or the planner has swapped away
-- from it — check `SHOW INDEX FROM <table>` and, if the index is
-- present, run `ANALYZE TABLE <table>` to refresh statistics.

SELECT '─── §F1 FindBySpecMatch (needs sql/08 index) ──────────' AS section;

EXPLAIN
SELECT DISTINCT a.legacyArticleId, a.articleNumber
FROM articlecriteria ac
JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
WHERE ac.criteriaDescription = 'Thread Size'
  AND ac.rawValue = 'M20 x 1.5'
LIMIT 10;

SELECT '─── §F2 FindSpecifications (needs sql/07 index) ──────' AS section;

EXPLAIN
SELECT criteriaDescription, rawValue
FROM articlecriteria
WHERE legacyArticleId = 12345
  AND criteriaDescription IN ('Length [mm]', 'Weight [kg]', 'Height [mm]');

SELECT '─── §F3 SearchCrossReferences (needs sql/06 index) ───' AS section;

EXPLAIN
SELECT id, oemNumber, brandName, legacyArticleId
FROM articlecrosses
WHERE oemNumberNormalized = '263202g000';

SELECT '─── §F4 SearchByOEM primary (needs sql/06 index) ─────' AS section;

EXPLAIN
SELECT id, number, articleId
FROM oem_number
WHERE clean_number = '263202g000';

SELECT '─── §F5 PartsForVehicle (linkingTargetId index) ──────' AS section;

EXPLAIN
SELECT avt.legacyArticleId, avt.assemblyGroupNodeId
FROM articlesvehicletrees avt
WHERE avt.linkingTargetId = 30001
  AND avt.linkingTargetType = 'P'
LIMIT 100;

SELECT '─── §F6 SearchByOEMIndex (idx_normalized) ────────────' AS section;

EXPLAIN
SELECT legacyArticleId
FROM oem_search_index
WHERE normalized = '263202g000';


SELECT '' AS ' ';
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '  BASELINE COMPLETE — every §A row should say PRESENT + every §F row  ' AS ' ';
SELECT '  should show type=ref (not type=ALL) with the expected key.          ' AS ' ';
SELECT '  Any deviation = migration regression → paste output back.           ' AS ' ';
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
