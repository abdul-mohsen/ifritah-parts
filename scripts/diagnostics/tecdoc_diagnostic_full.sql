-- ============================================================================
-- TecDoc Audit + Diagnostic — REMAINING (unresolved after 2026-08-27 run)
-- ============================================================================
-- Every query already answered has been crossed off. Full history of
-- pinned facts lives locally in:
--
--   C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\tecdoc-known-answers\ANSWERED.md
--
-- What ran successfully on prior sessions:
--   §A1 — P0 index PASS/FAIL: all 5 PRESENT (sql/06 + sql/07 + sql/08)
--   §B1 — sampled distinct-linkage stats
--   §B2 — supersession HK coverage
--   §B3, §B3b — HK vehicle catalog (Sonata VIII linkage IDs 139268/139269)
--   §B4  — linkagetargets language coverage (en=307K, ru=306K, bg=305K)
--   §B4b — ambrand language coverage (EN=1785)
--   §C1  — REAL aftermarket brands per HK prefix (BOSCH/BREMBO/VALEO/HELLA/TEXTAR/etc. — all present)
--   §C2  — Marquee brand probe (every expected aftermarket brand confirmed)
--   §D1  — corpus size (19)
--   §D2  — corpus via oem_number (6/19 resolve — 33%)
--
-- What's left (blocked on collation-mismatch fix, now applied):
--   §D3  — corpus crossref counts per OEM (via articlecrosses)
--   §D4  — REAL aftermarket brand per corpus OEM
--   §D4b — brands rolled up per part_kind
--   §D5  — spec coverage per corpus OEM
--   §F1-§F6 — EXPLAIN plans (validates sql/06/07/08 hit by planner)
--
-- Runtime: ~30 seconds. All queries indexed-equality / bounded.
--
-- Usage:
--
--   mysql --host=<tecdoc-mysql-host> --user=<user> --password \
--         --database=<tecdoc-db-name> \
--         < scripts/diagnostics/tecdoc_diagnostic_full.sql \
--         > tecdoc-remaining-$(date +%Y-%m-%d).txt
--
-- MariaDB 10.3+ AND MySQL 5.7 / 8.x compatible.
-- ============================================================================


SELECT '' AS ' ';
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '  TECDOC DIAGNOSTIC — REMAINING queries only (§D3+ and §F)' AS ' ';
SELECT '  Run at:' AS ' ', NOW() AS run_at;
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '' AS ' ';


-- ═══════════════════════════════════════════════════════════════════════════
--         PART D — 19-OEM audit corpus verification (§D3+ only)
-- ═══════════════════════════════════════════════════════════════════════════
--
-- §D1 (temp-table load) and §D2 (oem_number lookup) already ran. We
-- reload the corpus here because MEMORY temp tables are session-scoped;
-- there's no way to reference results from an earlier session.
--
-- CHARACTER SET + COLLATE on `normalized`: without it, MySQL 8 defaults
-- new MEMORY tables to utf8mb4_0900_ai_ci which does not equal
-- utf8mb4_unicode_ci (TecDoc's collation) → ERROR 1267 on JOIN. Forcing
-- utf8mb4_unicode_ci here + adding COLLATE inline on each JOIN below
-- gives belt-and-suspenders safety across any MySQL/MariaDB version.

SELECT '─── §D1 Reload 19-OEM audit corpus temp table ────────' AS section;

DROP TEMPORARY TABLE IF EXISTS tmp_corpus;
CREATE TEMPORARY TABLE tmp_corpus (
	oem        VARCHAR(30) NOT NULL,
	normalized VARCHAR(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
	part_kind  VARCHAR(20) NOT NULL,
	PRIMARY KEY (normalized)
) ENGINE=MEMORY;

INSERT INTO tmp_corpus (oem, normalized, part_kind) VALUES
	('263202G000', '263202g000', 'oil_filter'),
	('263003C100', '263003c100', 'oil_filter'),
	('263003C300', '263003c300', 'oil_filter'),
	('263203CAA0', '263203caa0', 'oil_filter'),
	('263203CAB0', '263203cab0', 'oil_filter'),
	('581012GA10', '581012ga10', 'brake_pad'),
	('581012MA00', '581012ma00', 'brake_pad'),
	('581013QA50', '581013qa50', 'brake_pad'),
	('581012HA10', '581012ha10', 'brake_pad'),
	('581012HA20', '581012ha20', 'brake_pad'),
	('971332S000', '971332s000', 'cabin_filter'),
	('971332S100', '971332s100', 'cabin_filter'),
	('971331Y000', '971331y000', 'cabin_filter'),
	('971332J000', '971332j000', 'cabin_filter'),
	('821013SA00', '821013sa00', 'window'),
	('823703SA00', '823703sa00', 'window'),
	('822013SA00', '822013sa00', 'window'),
	('292202G010', '292202g010', 'oil_pan_gasket'),
	('292202G100', '292202g100', 'oil_pan_gasket');

SELECT COUNT(*) AS corpus_size FROM tmp_corpus;


-- ─── §D3 Corpus lookup via articlecrosses + brand diversity ────────

SELECT '─── §D3 Corpus crossref counts + brand diversity ─────' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(acr.id)                 AS crossref_rows,
	COUNT(DISTINCT acr.brandName) AS distinct_brands
FROM tmp_corpus c
LEFT JOIN articlecrosses acr
	ON acr.oemNumberNormalized = c.normalized COLLATE utf8mb4_unicode_ci
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;


-- ─── §D4 REAL aftermarket brand per corpus OEM (correct JOIN) ──────

SELECT '─── §D4 REAL aftermarket brand per corpus OEM ────────' AS section;

SELECT
	c.oem,
	c.part_kind,
	amb.brandName AS aftermarket_brand,
	COUNT(*)      AS rows_for_this_oem_brand
FROM tmp_corpus c
JOIN articlecrosses acr
	ON acr.oemNumberNormalized = c.normalized COLLATE utf8mb4_unicode_ci
JOIN articles      a   ON a.legacyArticleId = acr.legacyArticleId
JOIN ambrand       amb ON amb.brandId = a.dataSupplierId
                      AND amb.lang = 'EN'
GROUP BY c.oem, c.part_kind, amb.brandName
ORDER BY c.part_kind, c.oem, rows_for_this_oem_brand DESC;


SELECT '─── §D4b Aftermarket brands rolled up per part_kind ──' AS section;

SELECT
	c.part_kind,
	amb.brandName                 AS aftermarket_brand,
	COUNT(DISTINCT c.oem)         AS distinct_corpus_oems_covered,
	COUNT(*)                      AS total_crossref_rows
FROM tmp_corpus c
JOIN articlecrosses acr
	ON acr.oemNumberNormalized = c.normalized COLLATE utf8mb4_unicode_ci
JOIN articles      a   ON a.legacyArticleId = acr.legacyArticleId
JOIN ambrand       amb ON amb.brandId = a.dataSupplierId
                      AND amb.lang = 'EN'
GROUP BY c.part_kind, amb.brandName
ORDER BY c.part_kind, distinct_corpus_oems_covered DESC, total_crossref_rows DESC;


-- ─── §D5 Spec coverage per corpus OEM (validates sql/07 index) ─────

SELECT '─── §D5 Spec coverage per corpus OEM ─────────────────' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(ac.id)                          AS spec_rows,
	COUNT(DISTINCT ac.criteriaDescription) AS distinct_criteria
FROM tmp_corpus c
JOIN articlecrosses acr
	ON acr.oemNumberNormalized = c.normalized COLLATE utf8mb4_unicode_ci
JOIN articlecriteria ac ON ac.legacyArticleId = acr.legacyArticleId
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;

DROP TEMPORARY TABLE IF EXISTS tmp_corpus;


-- ═══════════════════════════════════════════════════════════════════════════
--         PART F — EXPLAIN plans (validates every hot query hits an index)
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Every EXPLAIN below should show `type=ref` (or better) with a
-- meaningful `key`. §A1 already confirmed all 5 P0 indexes are PRESENT,
-- so we expect every EXPLAIN to hit the right one.

SELECT '─── §F1 EXPLAIN — articlecriteria FindBySpecMatch ────' AS section;

EXPLAIN
SELECT DISTINCT a.legacyArticleId, a.articleNumber
FROM articlecriteria ac
JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
WHERE ac.criteriaDescription = 'Thread Size'
  AND ac.rawValue = 'M20 x 1.5'
LIMIT 10;

SELECT '─── §F2 EXPLAIN — articlecriteria FindSpecifications ─' AS section;

EXPLAIN
SELECT criteriaDescription, rawValue
FROM articlecriteria
WHERE legacyArticleId = 12345
  AND criteriaDescription IN ('Length [mm]', 'Weight [kg]', 'Height [mm]');

SELECT '─── §F3 EXPLAIN — articlecrosses SearchCrossReferences ┈' AS section;

EXPLAIN
SELECT id, oemNumber, brandName, legacyArticleId
FROM articlecrosses
WHERE oemNumberNormalized = '263202g000';

SELECT '─── §F4 EXPLAIN — oem_number SearchByOEM primary ─────' AS section;

EXPLAIN
SELECT id, number, articleId
FROM oem_number
WHERE clean_number = '263202g000';

SELECT '─── §F5 EXPLAIN — articlesvehicletrees PartsForVehicle ┈' AS section;

EXPLAIN
SELECT avt.legacyArticleId, avt.assemblyGroupNodeId
FROM articlesvehicletrees avt
WHERE avt.linkingTargetId = 30001
  AND avt.linkingTargetType = 'P'
LIMIT 100;

SELECT '─── §F6 EXPLAIN — oem_search_index SearchByOEMIndex ──' AS section;

EXPLAIN
SELECT legacyArticleId
FROM oem_search_index
WHERE normalized = '263202g000';


-- ═══════════════════════════════════════════════════════════════════════════
--                     REPORT COMPLETE
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '' AS ' ';
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '  REMAINING DIAGNOSTIC COMPLETE — paste output back for final analysis' AS ' ';
SELECT '  Completed at:' AS ' ', NOW() AS completed_at;
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
