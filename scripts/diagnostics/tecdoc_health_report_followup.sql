-- ============================================================================
-- TecDoc Health Report — FOLLOW-UP
-- ============================================================================
-- Run AFTER `tecdoc_health_report.sql`. This is not a replacement — it
-- only re-runs the sections that failed or came back empty in v1, and
-- adds new probes that v1 raised as new questions.
--
-- What v1 answered (do NOT re-run these):
--   * Section 1  table sizes                         ✓ answered
--   * Section 2  P0 index status                     ✓ answered
--   * Section 3  HK manuIds                          ✓ answered
--   * Section 5  articlecrosses HK-prefix coverage   ✓ answered
--   * Section 6  criteriaDescription distribution    ✓ answered
--
-- What v1 left open and this file answers:
--
--   FIX-4.  oem_number HK coverage
--           v1 output was empty because it JOINed
--           articles.mfrId -> manufacturers.manuId and that JOIN
--           returned 0 rows. This file bypasses that JOIN.
--
--   FIX-7.  articlesvehicletrees linkingTargetType
--           v1 errored on `COUNT(*) AS rows` (MySQL 8 reserved word).
--           Fixed to `AS row_count`.
--
--   FIX-8/9/10. Same "articles.mfrId JOIN" workaround for
--           supersession, linkagetargets, ambrand.
--
--   NEW-A.  articles + ambrand schema discovery
--           So we know the actual manufacturer/brand column names for
--           future code changes.
--
--   NEW-B.  EXPLICIT aftermarket-brand probe
--           v1 section 5 revealed the top-30 brands on HK-prefix
--           cross-refs are all car OEMs — no Bosch/Mann/Mahle/etc.
--           This section confirms with a per-brand pivot.
--
--   NEW-C.  Test-corpus OEM lookup
--           Take 19 real HK OEMs from the audit set and verify each
--           has data in oem_number and/or articlecrosses.
--
--   NEW-D.  EXPLAIN plans on hot production queries.
--
-- Usage:
--
--   mysql --host=<your-tecdoc-mysql> \
--         --user=<user> --password \
--         --database=<tecdoc-db-name> \
--         < scripts/diagnostics/tecdoc_health_report_followup.sql \
--         > tecdoc-followup-2026-XX-XX.txt
--
-- Runtime: 2-8 minutes.
-- ============================================================================


-- ═══════════════════════════════════════════════════════════════════════════
-- NEW-A - ARTICLES + AMBRAND SCHEMA DISCOVERY
-- ═══════════════════════════════════════════════════════════════════════════
-- v1 assumed articles.mfrId links to manufacturers.manuId, but the
-- JOIN returned 0 rows. This section dumps the actual column layout
-- so we can identify the real linking column.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== NEW-A: articles + ambrand schema ===' AS section;

SELECT '--- articles columns ---' AS note;

SELECT
	COLUMN_NAME,
	DATA_TYPE,
	IS_NULLABLE,
	COLUMN_KEY,
	COLUMN_COMMENT
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'articles'
ORDER BY ORDINAL_POSITION;

SELECT '--- sample articles rows ---' AS note;

SELECT *
FROM articles
LIMIT 3;

SELECT '--- ambrand columns ---' AS note;

SELECT
	COLUMN_NAME,
	DATA_TYPE,
	IS_NULLABLE,
	COLUMN_KEY
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'ambrand'
ORDER BY ORDINAL_POSITION;

SELECT '--- sample ambrand rows (lang=en) ---' AS note;

SELECT *
FROM ambrand
WHERE lang = 'en'
LIMIT 10;

-- articlecriteria columns — confirms whether criteriaDescription + rawValue
-- are TEXT (need index prefix length) or VARCHAR (can index directly).
-- The 2026-08-26 sql/08 migration hit ERROR 1170 on this table, which
-- means at least one of these columns is TEXT/BLOB.
SELECT '--- articlecriteria columns (index-prefix implications) ---' AS note;

SELECT
	COLUMN_NAME,
	DATA_TYPE,
	CHARACTER_MAXIMUM_LENGTH,
	IS_NULLABLE,
	COLUMN_KEY,
	CASE
		WHEN DATA_TYPE IN ('text','mediumtext','longtext','tinytext','blob') THEN 'NEEDS_PREFIX'
		WHEN DATA_TYPE IN ('varchar','char') THEN 'DIRECT_INDEXABLE'
		ELSE 'N/A'
	END AS index_treatment
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'articlecriteria'
ORDER BY ORDINAL_POSITION;


-- ═══════════════════════════════════════════════════════════════════════════
-- FIX-4 - oem_number HK COVERAGE (JOIN-free)
-- ═══════════════════════════════════════════════════════════════════════════
-- Bypass articles.mfrId JOIN — probe oem_number.clean_number prefix
-- distribution directly (this is what the app actually queries).
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FIX-4: oem_number HK coverage (no JOIN) ===' AS section;

-- Total rows + distinct 2-char prefixes
SELECT
	COUNT(*)                              AS total_rows,
	COUNT(DISTINCT LEFT(clean_number, 2)) AS distinct_prefix2
FROM oem_number;

-- Rows for known HK prefix regions
SELECT
	LEFT(clean_number, 2) AS prefix2,
	COUNT(*)              AS rows_at_prefix
FROM oem_number
WHERE LEFT(clean_number, 2) IN
	('26','28','29','51','52','54','55','58','82','83','84','85','86',
	 '92','93','94','95','96','97','98','99')
GROUP BY prefix2
ORDER BY rows_at_prefix DESC;

-- Sample rows so we see actual clean_number format
SELECT
	number,
	clean_number,
	articleId
FROM oem_number
WHERE clean_number LIKE '26%'
   OR clean_number LIKE '58%'
   OR clean_number LIKE '97%'
LIMIT 15;


-- ═══════════════════════════════════════════════════════════════════════════
-- NEW-B - EXPLICIT AFTERMARKET-BRAND PROBE
-- ═══════════════════════════════════════════════════════════════════════════
-- v1 section 5 output showed top-30 brands on HK-prefix cross-refs
-- are all car OEMs (Hyundai/Kia/Subaru/Nissan/Honda/Toyota/etc.), NO
-- true aftermarket brands. This section confirms with a targeted probe.
--
-- Answer decides whether M2 (aftermarket richness) is achievable from
-- TecDoc alone or requires M4 (external aftermarket sources: RockAuto,
-- regional suppliers, community).
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== NEW-B: aftermarket-brand probe ===' AS section;

-- Per-major-aftermarket-brand rows across HK prefixes 26/58/82/97
SELECT
	brand,
	SUM(CASE WHEN prefix2 = '26' THEN rows_at_prefix ELSE 0 END) AS `26_filters`,
	SUM(CASE WHEN prefix2 = '58' THEN rows_at_prefix ELSE 0 END) AS `58_brakes`,
	SUM(CASE WHEN prefix2 = '82' THEN rows_at_prefix ELSE 0 END) AS `82_glass`,
	SUM(CASE WHEN prefix2 = '97' THEN rows_at_prefix ELSE 0 END) AS `97_hvac`,
	SUM(rows_at_prefix)                                          AS total
FROM (
	SELECT
		brandName                     AS brand,
		LEFT(oemNumberNormalized, 2)  AS prefix2,
		COUNT(*)                      AS rows_at_prefix
	FROM articlecrosses
	WHERE brandName IN (
	    'BOSCH','MANN','MANN-FILTER','MAHLE','DENSO','NGK','VALEO','HELLA',
	    'BREMBO','TEXTAR','FERODO','SACHS','FEBI','LEMFOERDER','LUK','INA',
	    'SKF','GATES','CONTINENTAL','KNECHT','FILTRON','WIX','PURFLUX',
	    'MAGNETI MARELLI','DELPHI','MEYLE','TRW','ATE','ZIMMERMANN'
	)
	  AND (
	    oemNumberNormalized LIKE '26%' OR oemNumberNormalized LIKE '58%'
	 OR oemNumberNormalized LIKE '82%' OR oemNumberNormalized LIKE '97%'
	  )
	GROUP BY brandName, LEFT(oemNumberNormalized, 2)
) t
GROUP BY brand
ORDER BY total DESC;

-- All non-car-OEM brands on any HK prefix (whitelist car OEMs out).
-- Anything that appears here is a potential aftermarket source in
-- this TecDoc dump.
SELECT
	brandName,
	COUNT(*) AS total_rows_across_hk_prefixes
FROM articlecrosses
WHERE (
	oemNumberNormalized LIKE '26%' OR oemNumberNormalized LIKE '58%' OR
	oemNumberNormalized LIKE '82%' OR oemNumberNormalized LIKE '97%'
  )
  AND brandName NOT IN (
	'HYUNDAI','KIA','GENESIS','HYUNDAI (BEIJING)','HYUNDAI (HUATAI)',
	'KIA (DYK)','TOYOTA','NISSAN','HONDA','MAZDA','SUBARU','SUZUKI',
	'MITSUBISHI','DAIHATSU','LEXUS','INFINITI','ISUZU','SSANGYONG',
	'CHRYSLER','DODGE','FORD','GM','OPEL','RENAULT','PEUGEOT',
	'CITROËN/PEUGEOT','BMW','MERCEDES-BENZ','VW','AUDI','GALLOPER',
	'JAC','HAWTAI','NISSAN (DFAC)','HITACHI','OM'
  )
GROUP BY brandName
ORDER BY total_rows_across_hk_prefixes DESC
LIMIT 30;


-- ═══════════════════════════════════════════════════════════════════════════
-- FIX-7 - articlesvehicletrees linkingTargetType (reserved-word fix)
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FIX-7: articlesvehicletrees linkingTargetType ===' AS section;

-- linkingTargetType distribution. `rows` was reserved in v1; `row_count` here.
SELECT
	linkingTargetType,
	COUNT(*) AS row_count
FROM articlesvehicletrees
WHERE linkingTargetType IN ('P','V','C','M','A','K','L','H','S','O')
GROUP BY linkingTargetType
ORDER BY row_count DESC;

-- Distinct linkage-target IDs per type. The app filters on 'P' —
-- if HK data uses another code, that filter drops everything.
SELECT
	linkingTargetType,
	COUNT(DISTINCT linkingTargetId) AS distinct_linkage_targets
FROM articlesvehicletrees
GROUP BY linkingTargetType
ORDER BY distinct_linkage_targets DESC;

-- Average rows per linkage for type='P' (first 100 samples)
SELECT
	'articlesvehicletrees rows per random P linkage' AS check_name,
	AVG(rows_per_linkage) AS avg_rows,
	MIN(rows_per_linkage) AS min_rows,
	MAX(rows_per_linkage) AS max_rows
FROM (
	SELECT linkingTargetId, COUNT(*) AS rows_per_linkage
	FROM articlesvehicletrees
	WHERE linkingTargetType = 'P'
	GROUP BY linkingTargetId
	LIMIT 100
) t;


-- ═══════════════════════════════════════════════════════════════════════════
-- FIX-8 - SUPERSESSION HK COVERAGE (JOIN-free)
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FIX-8: supersession HK coverage (no JOIN) ===' AS section;

-- Schema first — v1 was blind to the actual columns
SELECT
	TABLE_NAME,
	COLUMN_NAME,
	DATA_TYPE
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name IN ('replacedbyarticles','replacesarticles')
ORDER BY TABLE_NAME, ORDINAL_POSITION;

SELECT '--- sample replacedbyarticles ---' AS note;
SELECT * FROM replacedbyarticles LIMIT 5;

SELECT '--- sample replacesarticles ---' AS note;
SELECT * FROM replacesarticles LIMIT 5;

-- HK supersession rows via articlecrosses (bypass manufacturers JOIN)
SELECT
	'replacedbyarticles HK' AS tbl,
	COUNT(*)                AS hk_rows
FROM replacedbyarticles rba
WHERE rba.legacyArticleId IN (
	SELECT DISTINCT legacyArticleId
	FROM articlecrosses
	WHERE oemNumberNormalized LIKE '26%'
	   OR oemNumberNormalized LIKE '58%'
	   OR oemNumberNormalized LIKE '82%'
	   OR oemNumberNormalized LIKE '97%'
	LIMIT 100000
)
UNION ALL SELECT
	'replacesarticles HK',
	COUNT(*)
FROM replacesarticles ra
WHERE ra.legacyArticleId IN (
	SELECT DISTINCT legacyArticleId
	FROM articlecrosses
	WHERE oemNumberNormalized LIKE '26%'
	   OR oemNumberNormalized LIKE '58%'
	   OR oemNumberNormalized LIKE '82%'
	   OR oemNumberNormalized LIKE '97%'
	LIMIT 100000
);


-- ═══════════════════════════════════════════════════════════════════════════
-- FIX-9 - linkagetargets + modelseries (uses manuIds from v1 Section 3)
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FIX-9: linkagetargets + modelseries HK ===' AS section;

-- v1 confirmed the HK manuIds directly:
--   183  Hyundai
--   184  Kia
--   4473 Genesis
--   3123 Hyundai (Beijing)
--   3127 Kia (DYK)
--   3128 Hyundai (Huatai)
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

-- Elantra linkage IDs (English rows)
SELECT
	lt.linkageTargetId,
	m.manuName,
	ms.modelname,
	lt.description,
	lt.beginYearMonth,
	lt.endYearMonth,
	lt.lang
FROM linkagetargets lt
JOIN modelseries ms ON lt.vehicleModelSeriesId = ms.modelId
JOIN manufacturers m ON m.manuId = ms.manuId
WHERE m.manuId = 183
  AND ms.modelname LIKE '%Elantra%'
ORDER BY lt.beginYearMonth DESC
LIMIT 10;

-- Sonata linkage IDs (English rows)
SELECT
	lt.linkageTargetId,
	m.manuName,
	ms.modelname,
	lt.description,
	lt.beginYearMonth,
	lt.endYearMonth,
	lt.lang
FROM linkagetargets lt
JOIN modelseries ms ON lt.vehicleModelSeriesId = ms.modelId
JOIN manufacturers m ON m.manuId = ms.manuId
WHERE m.manuId = 183
  AND ms.modelname LIKE '%Sonata%'
ORDER BY lt.beginYearMonth DESC
LIMIT 10;


-- ═══════════════════════════════════════════════════════════════════════════
-- FIX-10 - LANGUAGE COVERAGE (reserved-word fix)
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FIX-10: language coverage ===' AS section;

SELECT 'linkagetargets' AS tbl, lang, COUNT(*) AS row_count
FROM linkagetargets
GROUP BY lang
ORDER BY row_count DESC
LIMIT 20;

SELECT 'ambrand' AS tbl, lang, COUNT(*) AS row_count
FROM ambrand
GROUP BY lang
ORDER BY row_count DESC
LIMIT 20;

SELECT 'assemblygroupnodenames' AS tbl, lang, COUNT(*) AS row_count
FROM assemblygroupnodenames
GROUP BY lang
ORDER BY row_count DESC
LIMIT 20;


-- ═══════════════════════════════════════════════════════════════════════════
-- NEW-C - TEST-CORPUS OEM LOOKUP
-- ═══════════════════════════════════════════════════════════════════════════
-- 19 real HK OEMs from the 1490-corpus audit set. Verifies each has data
-- behind it in both the primary path (oem_number) and the aftermarket
-- path (articlecrosses).
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== NEW-C: test-corpus OEM lookup ===' AS section;

CREATE TEMPORARY TABLE tmp_corpus_oems (oem VARCHAR(30));
INSERT INTO tmp_corpus_oems (oem) VALUES
  ('263202G000'),  -- Oil filter, Hyundai Sonata/Elantra
  ('263003C100'),  -- Oil filter, Hyundai Sonata V6
  ('263003C300'),  -- Oil filter, Kia
  ('263203CAA0'),
  ('263203CAB0'),
  ('581012GA10'),  -- Brake pad, front, Hyundai Sonata YF
  ('581012MA00'),  -- Brake pad, Kia
  ('581013QA50'),
  ('581012HA10'),
  ('581012HA20'),
  ('971332S000'),  -- Cabin filter, Hyundai Tucson
  ('971332S100'),
  ('971331Y000'),
  ('971332J000'),
  ('821013SA00'),  -- Window regulator/motor
  ('823703SA00'),
  ('822013SA00'),
  ('292202G010'),  -- Oil pan gasket
  ('292202G100');

SELECT '--- via oem_number (primary path) ---' AS note;

SELECT
	c.oem                                                                        AS corpus_oem,
	LOWER(REPLACE(REPLACE(REPLACE(c.oem, '-', ''), ' ', ''), '.', ''))           AS normalized,
	COUNT(on2.id)                                                                AS oem_number_rows
FROM tmp_corpus_oems c
LEFT JOIN oem_number on2
  ON on2.clean_number = LOWER(REPLACE(REPLACE(REPLACE(c.oem, '-', ''), ' ', ''), '.', ''))
GROUP BY c.oem
ORDER BY oem_number_rows DESC;

SELECT '--- via articlecrosses (aftermarket path) ---' AS note;

SELECT
	c.oem                                          AS corpus_oem,
	COUNT(acr.id)                                  AS crossref_rows,
	COUNT(DISTINCT acr.brandName)                  AS distinct_brands,
	GROUP_CONCAT(DISTINCT acr.brandName
	             ORDER BY acr.brandName SEPARATOR ', ') AS brands
FROM tmp_corpus_oems c
LEFT JOIN articlecrosses acr
  ON acr.oemNumberNormalized = LOWER(REPLACE(REPLACE(REPLACE(c.oem, '-', ''), ' ', ''), '.', ''))
GROUP BY c.oem
ORDER BY crossref_rows DESC;

DROP TEMPORARY TABLE tmp_corpus_oems;


-- ═══════════════════════════════════════════════════════════════════════════
-- NEW-D - EXPLAIN plans on production queries
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== NEW-D: EXPLAIN plans ===' AS section;

SELECT '--- EXPLAIN 1: articlecriteria (FindSpecifications) ---' AS note;

EXPLAIN
SELECT criteriaDescription, rawValue
FROM articlecriteria
WHERE legacyArticleId = 12345
  AND criteriaDescription IN ('Length [mm]','Weight [kg]','Height [mm]');

SELECT '--- EXPLAIN 2: articlecrosses (SearchCrossReferences) ---' AS note;

EXPLAIN
SELECT id, oemNumber, brandName, legacyArticleId
FROM articlecrosses
WHERE oemNumberNormalized = '263202g000';

SELECT '--- EXPLAIN 3: oem_number (SearchByOEM primary) ---' AS note;

EXPLAIN
SELECT id, number, articleId
FROM oem_number
WHERE clean_number = '263202g000';

SELECT '--- EXPLAIN 4: articlesvehicletrees (PartsForVehicle) ---' AS note;

EXPLAIN
SELECT avt.legacyArticleId, avt.assemblyGroupNodeId
FROM articlesvehicletrees avt
WHERE avt.linkingTargetId = 30001
  AND avt.linkingTargetType = 'P'
LIMIT 100;

SELECT '--- EXPLAIN 5: oem_search_index (SearchByOEMIndex 3rd-level) ---' AS note;

EXPLAIN
SELECT legacyArticleId
FROM oem_search_index
WHERE normalized = '263202g000';


-- ═══════════════════════════════════════════════════════════════════════════
-- FOLLOW-UP REPORT COMPLETE
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FOLLOW-UP COMPLETE — paste full output back to the agent ===' AS final;
