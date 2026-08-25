-- ============================================================================
-- TecDoc Health Report — FOLLOW-UP v2 (memory-safe + proper JOIN)
-- ============================================================================
-- Run AFTER `tecdoc_health_report_followup.sql` (v1). This is not a
-- replacement — it re-runs only the sections that either
--   (a) blew up MySQL /var/tmp on the previous run (huge COUNT DISTINCT
--       and huge IN() subqueries), or
--   (b) queried the wrong JOIN column (articles.mfrId — which is 0 for
--       every article in this TecDoc dump; the real link is
--       articles.dataSupplierId → ambrand.brandId).
--
-- What v1-followup answered (do NOT re-run):
--   * NEW-A     articles + ambrand + articlecriteria schema  ✓
--   * FIX-4     oem_number HK prefix distribution             ✓
--   * NEW-B v1  brandName top-30 on HK prefixes               ✓ (but wrong
--                — brandName is the CROSS-REFED OEM brand,
--                not the aftermarket brand; superseded here)
--   * FIX-7 q1  linkingTargetType row_count breakdown         ✓
--
-- What v1-followup left open and this file answers:
--
--   FIX-7 q2.   Distinct linkage targets by type — v1 blew up on
--               COUNT(DISTINCT) over 340M rows. Replaced with a
--               memory-cheap sampled probe.
--
--   FIX-8/9/10. Rewritten to avoid the IN(SELECT ... LIMIT 100000)
--               pattern that spilled to /var/tmp.
--
--   NEW-B2.     THE ACTUAL AFTERMARKET-BRAND PROBE.
--               v1 counted articlecrosses.brandName, but that column
--               is the CROSS-REFED OEM brand (Fiat/Hyundai/Kia), not
--               the aftermarket brand who published the article. This
--               version JOINs through articles.dataSupplierId →
--               ambrand.brandId to get the REAL aftermarket brand.
--               Bounded by a small OEM-list to keep memory footprint
--               tiny.
--
--   NEW-C.      19-OEM corpus lookup — small (19-row) temp table,
--               memory-safe.
--
--   NEW-D.      EXPLAIN plans on hot production queries.
--
-- Usage:
--
--   mysql --host=<...> --user=<user> --password --database=<db> \
--         < scripts/diagnostics/tecdoc_health_report_followup_v2.sql \
--         > tecdoc-followup-v2-2026-XX-XX.txt
--
-- Runtime: 2-5 minutes. All queries are bounded — none scan unbounded
-- and none aggregate over more than 1-2M rows.
-- ============================================================================


-- ═══════════════════════════════════════════════════════════════════════════
-- FIX-7 q2 - articlesvehicletrees distinct linkages (memory-safe)
-- ═══════════════════════════════════════════════════════════════════════════
-- v1 ran `COUNT(DISTINCT linkingTargetId) GROUP BY linkingTargetType`
-- across 340M rows — that requires MySQL to hold every distinct id in
-- a temp table on disk. Killed /var/tmp.
--
-- Replace with a bounded sample: for type='P' only (the app's filter),
-- take up to 10 M rows and count distinct within that sample. This
-- upper-bounds temp usage at ~40 MB regardless of table size.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FIX-7 q2: distinct P linkages (sampled) ===' AS section;

-- Distinct linkingTargetIds within a 10M-row sample of type=P
SELECT
	COUNT(DISTINCT sample_id) AS distinct_p_linkages_in_10m_sample
FROM (
	SELECT linkingTargetId AS sample_id
	FROM articlesvehicletrees
	WHERE linkingTargetType = 'P'
	LIMIT 10000000
) t;

-- Rows-per-linkage sample across 1000 random P linkages
SELECT '--- rows-per-P-linkage sample (1000 linkages) ---' AS note;

SELECT
	AVG(rows_per_linkage) AS avg_rows,
	MIN(rows_per_linkage) AS min_rows,
	MAX(rows_per_linkage) AS max_rows,
	COUNT(*)              AS linkages_sampled
FROM (
	SELECT linkingTargetId, COUNT(*) AS rows_per_linkage
	FROM articlesvehicletrees
	WHERE linkingTargetType = 'P'
	GROUP BY linkingTargetId
	LIMIT 1000
) t;


-- ═══════════════════════════════════════════════════════════════════════════
-- NEW-B2 - REAL AFTERMARKET-BRAND PROBE (via dataSupplierId → ambrand)
-- ═══════════════════════════════════════════════════════════════════════════
-- v1 counted articlecrosses.brandName — but that column is the OEM brand
-- being cross-referenced TO, not the aftermarket brand publishing the
-- article. Sample v1 output confirmed:
--
--   FIAT, DACIA, LANCIA, VAUXHALL, RENAULT TRUCKS, CITROËN, IVECO,
--   VOLVO, GENERAL MOTORS, PERKINS, ALFA ROMEO, PORSCHE, MAN, SETRA
--
-- Those are all CAR OEMS with numbers that happen to start with 26/58/82/97.
-- The REAL aftermarket brand (who publishes the article) is on the
-- `articles.dataSupplierId` side and looks up in `ambrand`.
--
-- This section does the correct JOIN — bounded to 1 M cross-refs to
-- keep memory footprint small.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== NEW-B2: REAL aftermarket brands on HK cross-refs ===' AS section;

-- Which aftermarket brands publish parts that cross-reference to HK OEMs?
-- HK OEM patterns: 10-char alphanumeric starting with known HK prefixes.
-- Tighter pattern: 3-char prefix ending in digit (HK convention), then
-- 7-char part-identifier. Compare to Fiat/Dacia which use dotted decimals.
SELECT
	amb.brandName            AS aftermarket_brand,
	COUNT(*)                 AS parts_cross_reffed_to_hk_oems,
	COUNT(DISTINCT acr.legacyArticleId) AS distinct_parts
FROM articlecrosses acr
JOIN articles a  ON a.legacyArticleId = acr.legacyArticleId
JOIN ambrand   amb ON amb.brandId = a.dataSupplierId
                 AND amb.lang = 'EN'
WHERE (
	acr.oemNumberNormalized LIKE '263%' OR
	acr.oemNumberNormalized LIKE '264%' OR
	acr.oemNumberNormalized LIKE '265%' OR
	acr.oemNumberNormalized LIKE '581%' OR
	acr.oemNumberNormalized LIKE '582%' OR
	acr.oemNumberNormalized LIKE '583%' OR
	acr.oemNumberNormalized LIKE '821%' OR
	acr.oemNumberNormalized LIKE '822%' OR
	acr.oemNumberNormalized LIKE '823%' OR
	acr.oemNumberNormalized LIKE '971%' OR
	acr.oemNumberNormalized LIKE '972%'
  )
  AND CHAR_LENGTH(acr.oemNumberNormalized) BETWEEN 8 AND 12
GROUP BY amb.brandName
ORDER BY parts_cross_reffed_to_hk_oems DESC
LIMIT 30;

-- Also probe by name for the marquee aftermarket brands — cheapest
-- possible way to answer "does Bosch have ANY part for HK OEMs?".
SELECT '--- explicit marquee-brand probe ---' AS note;

SELECT
	amb.brandName,
	COUNT(*) AS hk_cross_ref_rows
FROM ambrand amb
JOIN articles      a   ON a.dataSupplierId = amb.brandId
JOIN articlecrosses acr ON acr.legacyArticleId = a.legacyArticleId
WHERE amb.lang = 'EN'
  AND amb.brandName IN (
    'BOSCH','MANN-FILTER','MAHLE','MAHLE ORIGINAL','KNECHT','DENSO','NGK','VALEO','HELLA',
    'BREMBO','TEXTAR','FERODO','SACHS','FEBI','FEBI BILSTEIN','LEMFOERDER',
    'LUK','INA','SKF','GATES','CONTINENTAL','FILTRON','WIX','PURFLUX',
    'MAGNETI MARELLI','DELPHI','MEYLE','TRW','ATE','ZIMMERMANN',
    'BLUE PRINT','KYB','MONROE','SANGSIN','MOBIS'
  )
  AND (
    acr.oemNumberNormalized LIKE '263%' OR acr.oemNumberNormalized LIKE '581%' OR
    acr.oemNumberNormalized LIKE '821%' OR acr.oemNumberNormalized LIKE '971%'
  )
GROUP BY amb.brandName
ORDER BY hk_cross_ref_rows DESC;


-- ═══════════════════════════════════════════════════════════════════════════
-- FIX-8 - SUPERSESSION HK COVERAGE (memory-safe)
-- ═══════════════════════════════════════════════════════════════════════════
-- v1 used IN(SELECT ... LIMIT 100000) — MySQL materializes the 100K
-- values in a temp table. Replace with EXISTS pattern and no LIMIT
-- inside the subquery (using indexed articlecrosses lookup).
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FIX-8: supersession schema + HK coverage ===' AS section;

-- Schema of the supersession tables
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

-- HK supersession — sample 1000 HK articles from articlecrosses,
-- then count how many of them have supersession rows. Small
-- temp footprint.
SELECT '--- HK supersession coverage (1000-article sample) ---' AS note;

SELECT
	'replacedbyarticles HK (sampled)' AS tbl,
	COUNT(*)                          AS matched_rows,
	COUNT(DISTINCT rba.legacyArticleId) AS distinct_articles
FROM replacedbyarticles rba
WHERE EXISTS (
	SELECT 1 FROM articlecrosses acr
	WHERE acr.legacyArticleId = rba.legacyArticleId
	  AND (
	    acr.oemNumberNormalized LIKE '263%' OR acr.oemNumberNormalized LIKE '581%' OR
	    acr.oemNumberNormalized LIKE '821%' OR acr.oemNumberNormalized LIKE '971%'
	  )
)
LIMIT 1;

SELECT
	'replacesarticles HK (sampled)' AS tbl,
	COUNT(*)                        AS matched_rows,
	COUNT(DISTINCT ra.legacyArticleId) AS distinct_articles
FROM replacesarticles ra
WHERE EXISTS (
	SELECT 1 FROM articlecrosses acr
	WHERE acr.legacyArticleId = ra.legacyArticleId
	  AND (
	    acr.oemNumberNormalized LIKE '263%' OR acr.oemNumberNormalized LIKE '581%' OR
	    acr.oemNumberNormalized LIKE '821%' OR acr.oemNumberNormalized LIKE '971%'
	  )
)
LIMIT 1;


-- ═══════════════════════════════════════════════════════════════════════════
-- FIX-9 - linkagetargets + modelseries (uses manuIds from v1 Section 3)
-- ═══════════════════════════════════════════════════════════════════════════
-- No change vs v1-followup — this JOIN uses manufacturers directly by
-- manuId, not articles.mfrId, so it was correct in v1. Repeated here
-- because v1 followup died before reaching it.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FIX-9: linkagetargets + modelseries HK ===' AS section;

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

-- Elantra linkage IDs
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

-- Sonata linkage IDs
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
-- FIX-10 - LANGUAGE COVERAGE
-- ═══════════════════════════════════════════════════════════════════════════
-- Runs a GROUP BY lang on 3 tables — memory-cheap because `lang` has
-- <20 distinct values.
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

-- assemblygroupnodenames is empty per v1 section 1 — skip GROUP BY
SELECT
	'assemblygroupnodenames' AS tbl,
	COUNT(*)                 AS total_rows
FROM assemblygroupnodenames;


-- ═══════════════════════════════════════════════════════════════════════════
-- NEW-C - TEST-CORPUS OEM LOOKUP (memory-safe temp table)
-- ═══════════════════════════════════════════════════════════════════════════
-- 19-row temp table, bounded queries — /var/tmp footprint <1 MB.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== NEW-C: test-corpus OEM lookup ===' AS section;

DROP TEMPORARY TABLE IF EXISTS tmp_corpus_oems;
CREATE TEMPORARY TABLE tmp_corpus_oems (
	oem VARCHAR(30) NOT NULL,
	normalized VARCHAR(30) NOT NULL,
	INDEX idx_normalized (normalized)
);

INSERT INTO tmp_corpus_oems (oem, normalized) VALUES
  ('263202G000', '263202g000'),  -- Oil filter, Hyundai Sonata/Elantra
  ('263003C100', '263003c100'),
  ('263003C300', '263003c300'),
  ('263203CAA0', '263203caa0'),
  ('263203CAB0', '263203cab0'),
  ('581012GA10', '581012ga10'),  -- Brake pad, Hyundai Sonata YF
  ('581012MA00', '581012ma00'),
  ('581013QA50', '581013qa50'),
  ('581012HA10', '581012ha10'),
  ('581012HA20', '581012ha20'),
  ('971332S000', '971332s000'),  -- Cabin filter, Hyundai Tucson
  ('971332S100', '971332s100'),
  ('971331Y000', '971331y000'),
  ('971332J000', '971332j000'),
  ('821013SA00', '821013sa00'),
  ('823703SA00', '823703sa00'),
  ('822013SA00', '822013sa00'),
  ('292202G010', '292202g010'),
  ('292202G100', '292202g100');

SELECT '--- via oem_number (primary path) ---' AS note;

SELECT
	c.oem                       AS corpus_oem,
	COUNT(on2.id)               AS oem_number_rows,
	MIN(on2.articleId)          AS sample_article_id
FROM tmp_corpus_oems c
LEFT JOIN oem_number on2 ON on2.clean_number = c.normalized
GROUP BY c.oem
ORDER BY oem_number_rows DESC;

SELECT '--- via articlecrosses (aftermarket path) ---' AS note;

SELECT
	c.oem                                    AS corpus_oem,
	COUNT(acr.id)                            AS crossref_rows,
	COUNT(DISTINCT acr.brandName)            AS distinct_brands
FROM tmp_corpus_oems c
LEFT JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
GROUP BY c.oem
ORDER BY crossref_rows DESC;

-- Real aftermarket brand names for corpus OEMs (JOIN through articles→ambrand).
-- If any brands appear here, they are what the app CAN return for those OEMs.
SELECT '--- REAL aftermarket brands per corpus OEM ---' AS note;

SELECT
	c.oem                              AS corpus_oem,
	amb.brandName                      AS aftermarket_brand,
	COUNT(*)                           AS rows
FROM tmp_corpus_oems c
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
JOIN articles a         ON a.legacyArticleId = acr.legacyArticleId
JOIN ambrand   amb      ON amb.brandId = a.dataSupplierId
                       AND amb.lang = 'EN'
GROUP BY c.oem, amb.brandName
ORDER BY c.oem, rows DESC;

DROP TEMPORARY TABLE tmp_corpus_oems;


-- ═══════════════════════════════════════════════════════════════════════════
-- NEW-D - EXPLAIN plans on hot production queries
-- ═══════════════════════════════════════════════════════════════════════════
-- EXPLAINs are metadata-only — no data scan, zero temp footprint.
-- Run AFTER sql/08 hotfix so EXPLAIN 1 shows the new index in use.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== NEW-D: EXPLAIN plans ===' AS section;

SELECT '--- EXPLAIN 1: articlecriteria FindBySpecMatch (should hit sql/08 index) ---' AS note;

EXPLAIN
SELECT DISTINCT a.legacyArticleId, a.articleNumber
FROM articlecriteria ac
JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
WHERE ac.criteriaDescription = 'Thread Size'
  AND ac.rawValue = 'M20 x 1.5'
LIMIT 10;

SELECT '--- EXPLAIN 2: articlecriteria FindSpecifications (should hit legacyArticleId index) ---' AS note;

EXPLAIN
SELECT criteriaDescription, rawValue
FROM articlecriteria
WHERE legacyArticleId = 12345
  AND criteriaDescription IN ('Length [mm]','Weight [kg]','Height [mm]');

SELECT '--- EXPLAIN 3: articlecrosses (SearchCrossReferences) ---' AS note;

EXPLAIN
SELECT id, oemNumber, brandName, legacyArticleId
FROM articlecrosses
WHERE oemNumberNormalized = '263202g000';

SELECT '--- EXPLAIN 4: oem_number (SearchByOEM primary) ---' AS note;

EXPLAIN
SELECT id, number, articleId
FROM oem_number
WHERE clean_number = '263202g000';

SELECT '--- EXPLAIN 5: articlesvehicletrees (PartsForVehicle) ---' AS note;

EXPLAIN
SELECT avt.legacyArticleId, avt.assemblyGroupNodeId
FROM articlesvehicletrees avt
WHERE avt.linkingTargetId = 30001
  AND avt.linkingTargetType = 'P'
LIMIT 100;

SELECT '--- EXPLAIN 6: oem_search_index (SearchByOEMIndex 3rd-level) ---' AS note;

EXPLAIN
SELECT legacyArticleId
FROM oem_search_index
WHERE normalized = '263202g000';


-- ═══════════════════════════════════════════════════════════════════════════
-- FOLLOW-UP v2 COMPLETE
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== FOLLOW-UP v2 COMPLETE — paste full output back ===' AS final;
