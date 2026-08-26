-- ============================================================================
-- TecDoc Health Report — MINIMAL v2 (fast, MariaDB + MySQL compatible)
-- ============================================================================
-- Replaces the earlier tecdoc_health_report_followup_v2.sql design that
-- did prefix-LIKE scans over 30M-row articlecrosses. This version uses
-- ONLY indexed equality lookups, bounded temp tables, and small
-- constant-list JOINs. Target total runtime: <30 seconds.
--
-- Compatibility:
--   * MariaDB 10.3+ ✓
--   * MySQL 5.7+ / 8.x ✓
--   * No window functions, no JSON_TABLE, no LATERAL, no FORMAT=TREE
--   * All queries use standard ANSI-ish SQL with only indexed columns
--     in the WHERE clause
--
-- Skips everything v1 + v1-followup already answered:
--   ✓ Table sizes, indexes, schema
--   ✓ HK manuIds (183/184/4473/3123/3127/3128)
--   ✓ articlecriteria column types (all TEXT(65535) except criteriaType)
--   ✓ articles.mfrId is 0 → JOIN through dataSupplierId → ambrand.brandId
--   ✓ linkingTargetType distribution (274M P out of 340M)
--
-- Answers the remaining questions with the CHEAPEST possible queries:
--
--   A. Do the 19 real HK corpus OEMs resolve? (indexed equality lookups)
--   B. What are the ACTUAL aftermarket brands for those OEMs?
--      (JOIN through articles.dataSupplierId → ambrand)
--   C. Do any of those articles have supersession chains?
--   D. Do the corpus OEMs have specs in articlecriteria?
--   E. Vehicle catalog: Hyundai/Kia/Genesis linkage IDs (indexed JOIN)
--   F. Language distribution (small tables)
--   G. EXPLAIN plans on hot production queries (post-sql/08 verification)
--
-- Usage:
--
--   mysql --host=<...> --user=<user> --password --database=<db> \
--         < scripts/diagnostics/tecdoc_health_report_min.sql \
--         > tecdoc-min-2026-XX-XX.txt
--
-- ============================================================================


-- ═══════════════════════════════════════════════════════════════════════════
-- SETUP - 19-row corpus temp table with indexed normalized column
-- ═══════════════════════════════════════════════════════════════════════════
-- Everything downstream uses this table for indexed equality lookups.
-- All ~19 JOINs will be single-row index seeks — total <1MB temp footprint.

DROP TEMPORARY TABLE IF EXISTS tmp_corpus;
CREATE TEMPORARY TABLE tmp_corpus (
	oem        VARCHAR(30) NOT NULL,
	normalized VARCHAR(30) NOT NULL,
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


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION A - Do the 19 corpus OEMs resolve via oem_number? (indexed)
-- ═══════════════════════════════════════════════════════════════════════════
-- Uses idx_oem_number_clean_number (sql/06). Each row = 1 index seek.
-- Time: <1 second.

SELECT '=== A: corpus OEM lookup via oem_number ===' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(on2.id)      AS oem_number_rows,
	MIN(on2.articleId) AS sample_article_id
FROM tmp_corpus c
LEFT JOIN oem_number on2 ON on2.clean_number = c.normalized
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION B - Real aftermarket brands per corpus OEM
-- ═══════════════════════════════════════════════════════════════════════════
-- The CORRECT aftermarket-brand probe. JOIN chain:
--   tmp_corpus → articlecrosses (via oemNumberNormalized, indexed)
--                → articles (via legacyArticleId, PK)
--                → ambrand (via dataSupplierId = brandId, indexed)
--
-- All JOINs on indexed columns. Bounded by 19 corpus OEMs → typically
-- <500 total rows returned. Time: <2 seconds.

SELECT '=== B: crossref counts per corpus OEM ===' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(acr.id) AS crossref_rows
FROM tmp_corpus c
LEFT JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;

SELECT '=== B2: REAL aftermarket brands per corpus OEM ===' AS section;

-- Aftermarket brand names attached to each corpus OEM's cross-refs
SELECT
	c.oem,
	c.part_kind,
	amb.brandName AS aftermarket_brand,
	COUNT(*)      AS rows_for_this_oem_brand
FROM tmp_corpus c
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
JOIN articles      a   ON a.legacyArticleId = acr.legacyArticleId
JOIN ambrand       amb ON amb.brandId = a.dataSupplierId
                      AND amb.lang = 'EN'
GROUP BY c.oem, c.part_kind, amb.brandName
ORDER BY c.part_kind, c.oem, rows_for_this_oem_brand DESC;

-- Same thing rolled up per part_kind — one row per (part_kind, brand)
-- gives us the "does Bosch have oil filters for Hyundai?" summary.
SELECT '=== B3: aftermarket brands rolled up per part_kind ===' AS section;

SELECT
	c.part_kind,
	amb.brandName                       AS aftermarket_brand,
	COUNT(DISTINCT c.oem)               AS distinct_corpus_oems_covered,
	COUNT(*)                            AS total_crossref_rows
FROM tmp_corpus c
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
JOIN articles      a   ON a.legacyArticleId = acr.legacyArticleId
JOIN ambrand       amb ON amb.brandId = a.dataSupplierId
                      AND amb.lang = 'EN'
GROUP BY c.part_kind, amb.brandName
ORDER BY c.part_kind, distinct_corpus_oems_covered DESC, total_crossref_rows DESC;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION C - Supersession chain presence for corpus articles
-- ═══════════════════════════════════════════════════════════════════════════
-- For each corpus OEM's article, does it have supersession rows?
-- Bounded by ~500 articles → ~500 indexed lookups. Time: <2 seconds.

SELECT '=== C: supersession presence for corpus OEMs ===' AS section;

-- Distinct legacyArticleIds for corpus OEMs
DROP TEMPORARY TABLE IF EXISTS tmp_corpus_articles;
CREATE TEMPORARY TABLE tmp_corpus_articles (
	legacyArticleId BIGINT NOT NULL,
	PRIMARY KEY (legacyArticleId)
) ENGINE=MEMORY
SELECT DISTINCT acr.legacyArticleId
FROM tmp_corpus c
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized;

SELECT
	'corpus_articles' AS metric,
	COUNT(*)          AS count
FROM tmp_corpus_articles;

SELECT
	'replacedbyarticles matching corpus' AS metric,
	COUNT(*) AS matches
FROM replacedbyarticles rba
JOIN tmp_corpus_articles tca ON tca.legacyArticleId = rba.legacyArticleId;

SELECT
	'replacesarticles matching corpus' AS metric,
	COUNT(*) AS matches
FROM replacesarticles ra
JOIN tmp_corpus_articles tca ON tca.legacyArticleId = ra.legacyArticleId;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION D - Spec (articlecriteria) presence for corpus articles
-- ═══════════════════════════════════════════════════════════════════════════
-- Uses the sql/07 idx_articlecriteria_legacyArticleId index.
-- Bounded ~500 indexed lookups. Time: <3 seconds.

SELECT '=== D: spec coverage per corpus OEM ===' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(ac.id)                          AS spec_rows,
	COUNT(DISTINCT ac.criteriaDescription) AS distinct_criteria
FROM tmp_corpus c
JOIN articlecrosses acr    ON acr.oemNumberNormalized = c.normalized
JOIN articlecriteria ac    ON ac.legacyArticleId = acr.legacyArticleId
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;

-- Top 10 criteriaDescriptions that appear on corpus articles
SELECT '=== D2: top criteriaDescription for corpus articles ===' AS section;

SELECT
	ac.criteriaDescription AS spec_name,
	COUNT(*)               AS occurrences
FROM tmp_corpus_articles tca
JOIN articlecriteria ac ON ac.legacyArticleId = tca.legacyArticleId
GROUP BY ac.criteriaDescription
ORDER BY occurrences DESC
LIMIT 10;

DROP TEMPORARY TABLE IF EXISTS tmp_corpus_articles;
DROP TEMPORARY TABLE IF EXISTS tmp_corpus;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION E - Vehicle catalog for HK manufacturers
-- ═══════════════════════════════════════════════════════════════════════════
-- HK manuIds confirmed in v1 section 3: 183 Hyundai, 184 Kia,
-- 4473 Genesis, 3123 Hyundai(Beijing), 3127 Kia(DYK), 3128 Hyundai(Huatai)
-- All JOINs on indexed columns. Time: <3 seconds.

SELECT '=== E: HK vehicle catalog ===' AS section;

SELECT
	m.manuName,
	COUNT(DISTINCT ms.modelId)         AS model_count,
	COUNT(DISTINCT lt.linkageTargetId) AS linkage_count
FROM modelseries ms
JOIN manufacturers m ON m.manuId = ms.manuId
LEFT JOIN linkagetargets lt ON lt.vehicleModelSeriesId = ms.modelId
WHERE m.manuId IN (183, 184, 4473, 3123, 3127, 3128)
GROUP BY m.manuName
ORDER BY linkage_count DESC;

-- 5 Elantra + 5 Sonata linkage IDs so the audit has real vehicle IDs
SELECT '=== E2: sample Elantra + Sonata linkage IDs ===' AS section;

(SELECT
	lt.linkageTargetId,
	m.manuName,
	ms.modelname,
	lt.description,
	lt.beginYearMonth,
	lt.endYearMonth
FROM linkagetargets lt
JOIN modelseries ms ON lt.vehicleModelSeriesId = ms.modelId
JOIN manufacturers m ON m.manuId = ms.manuId
WHERE m.manuId = 183
  AND ms.modelname LIKE '%Elantra%'
  AND lt.lang = 'en'
ORDER BY lt.beginYearMonth DESC
LIMIT 5)
UNION ALL
(SELECT
	lt.linkageTargetId,
	m.manuName,
	ms.modelname,
	lt.description,
	lt.beginYearMonth,
	lt.endYearMonth
FROM linkagetargets lt
JOIN modelseries ms ON lt.vehicleModelSeriesId = ms.modelId
JOIN manufacturers m ON m.manuId = ms.manuId
WHERE m.manuId = 183
  AND ms.modelname LIKE '%Sonata%'
  AND lt.lang = 'en'
ORDER BY lt.beginYearMonth DESC
LIMIT 5);


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION F - Language distribution
-- ═══════════════════════════════════════════════════════════════════════════
-- Small tables (linkagetargets 900K, ambrand 35K). Time: <1 second.
-- Only 2 tables — assemblygroupnodenames is empty per v1.

SELECT '=== F: language distribution ===' AS section;

SELECT 'linkagetargets' AS tbl, lang, COUNT(*) AS row_count
FROM linkagetargets
GROUP BY lang
ORDER BY row_count DESC
LIMIT 10;

SELECT 'ambrand' AS tbl, lang, COUNT(*) AS row_count
FROM ambrand
GROUP BY lang
ORDER BY row_count DESC
LIMIT 10;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION G - EXPLAIN plans on hot queries (post-sql/08 verification)
-- ═══════════════════════════════════════════════════════════════════════════
-- EXPLAIN is metadata-only, no data scan. Time: instant.
-- Run AFTER applying sql/08 hotfix — EXPLAIN 1 should show
-- key='idx_articlecriteria_criteria_value'.

SELECT '=== G: EXPLAIN plans ===' AS section;

SELECT '--- G1: articlecriteria FindBySpecMatch (should use sql/08 index) ---' AS note;

EXPLAIN
SELECT DISTINCT a.legacyArticleId, a.articleNumber
FROM articlecriteria ac
JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
WHERE ac.criteriaDescription = 'Thread Size'
  AND ac.rawValue = 'M20 x 1.5'
LIMIT 10;

SELECT '--- G2: articlecriteria FindSpecifications (should use legacyArticleId index) ---' AS note;

EXPLAIN
SELECT criteriaDescription, rawValue
FROM articlecriteria
WHERE legacyArticleId = 12345
  AND criteriaDescription IN ('Length [mm]', 'Weight [kg]', 'Height [mm]');

SELECT '--- G3: articlecrosses SearchCrossReferences (should use sql/06 index) ---' AS note;

EXPLAIN
SELECT id, oemNumber, brandName, legacyArticleId
FROM articlecrosses
WHERE oemNumberNormalized = '263202g000';

SELECT '--- G4: oem_number SearchByOEM primary (should use sql/06 index) ---' AS note;

EXPLAIN
SELECT id, number, articleId
FROM oem_number
WHERE clean_number = '263202g000';

SELECT '--- G5: articlesvehicletrees PartsForVehicle ---' AS note;

EXPLAIN
SELECT avt.legacyArticleId, avt.assemblyGroupNodeId
FROM articlesvehicletrees avt
WHERE avt.linkingTargetId = 30001
  AND avt.linkingTargetType = 'P'
LIMIT 100;

SELECT '--- G6: oem_search_index (3rd-level fallback) ---' AS note;

EXPLAIN
SELECT legacyArticleId
FROM oem_search_index
WHERE normalized = '263202g000';


-- ═══════════════════════════════════════════════════════════════════════════
-- REPORT COMPLETE
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== MINIMAL v2 COMPLETE — paste output back ===' AS final;
