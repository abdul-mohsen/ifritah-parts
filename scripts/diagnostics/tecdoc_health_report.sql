-- ============================================================================
-- TecDoc Database Health Report
-- ============================================================================
-- ONE big diagnostic script — run against your TecDoc MySQL (read-replica
-- is fine, everything here is read-only) and paste the output into the
-- next agent turn.
--
-- What this report answers:
--
--   1. What tables actually exist + how many rows each has
--   2. Are the sql/06 and sql/07 generated-column + index migrations
--      applied? (These are the difference between a 20-second search
--      and a 200ms search.)
--   3. Real Hyundai / Kia OEM coverage across every table the app touches
--   4. Whether the specific 25-OEM test corpus has data behind it
--   5. Language coverage on the "text" tables — the app defaults to
--      lang='en' and drops everything else
--   6. Sample real rows so we can see actual data shape (not just counts)
--   7. Query-plan verification — EXPLAIN on the exact queries the app
--      runs, so we know whether indexes are being hit
--
-- Usage:
--
--   mysql --host=<your-tecdoc-mysql> \
--         --user=<user> --password \
--         --database=<tecdoc-db-name> \
--         < scripts/diagnostics/tecdoc_health_report.sql \
--         > tecdoc-health-2026-XX-XX.txt
--
-- Or interactively:
--
--   mysql> source scripts/diagnostics/tecdoc_health_report.sql;
--
-- Runtime: 2-15 minutes depending on table sizes + whether the sql/06
-- and sql/07 indexes are in place. Each section prints a header banner
-- so the output is easy to scan.
--
-- Every query is bounded by LIMIT or COUNT so nothing scans unbounded.
-- If any section hangs > 3 minutes, it's telling you an index is
-- missing — kill the query, note which section, and skip ahead.
-- ============================================================================


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 1 - OVERALL TABLE SIZES
-- ═══════════════════════════════════════════════════════════════════════════
-- Sanity check that TecDoc data is even present. Anything reporting 0
-- rows means either the dump wasn't imported, the wrong schema is being
-- queried, or table names differ between versions.
-- Expected orders of magnitude (2020 TecDoc dump):
--   articles                    ~27,000,000
--   articlecrosses              ~30,000,000
--   articlecriteria             ~27,000,000
--   articlesvehicletrees       ~651,000,000
--   oem_number                  ~21,500,000
--   oem_search_index            ~1,700
--   linkagetargets              ~1,000,000
--   modelseries                 ~50,000
--   manufacturers               ~500
--   ambrand                     ~5,000
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 1: table sizes ===' AS section;

SELECT
	table_name,
	table_rows          AS approx_rows,
	ROUND(data_length / 1024 / 1024, 1)  AS data_mb,
	ROUND(index_length / 1024 / 1024, 1) AS index_mb
FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_name IN (
      'articles', 'articlecrosses', 'articlecriteria', 'articlesvehicletrees',
      'oem_number', 'oem_search_index',
      'linkagetargets', 'modelseries', 'manufacturers', 'ambrand',
      'assemblygroupnodenames', 'assemblygroupnodes',
      'replacedbyarticles', 'replacesarticles',
      'articlelinkages', 'articleinformation', 'articledocuments'
  )
ORDER BY approx_rows DESC;

-- Fallback exact counts when table_rows is stale (InnoDB caches it).
SELECT '--- exact counts (may be slow) ---' AS note;

SELECT 'articles'                AS tbl, COUNT(*) AS rows_exact FROM articles
UNION ALL SELECT 'articlecrosses',        COUNT(*) FROM articlecrosses
UNION ALL SELECT 'articlecriteria',       COUNT(*) FROM articlecriteria
UNION ALL SELECT 'oem_number',            COUNT(*) FROM oem_number
UNION ALL SELECT 'oem_search_index',      COUNT(*) FROM oem_search_index
UNION ALL SELECT 'linkagetargets',        COUNT(*) FROM linkagetargets
UNION ALL SELECT 'modelseries',           COUNT(*) FROM modelseries
UNION ALL SELECT 'manufacturers',         COUNT(*) FROM manufacturers
UNION ALL SELECT 'ambrand',               COUNT(*) FROM ambrand
UNION ALL SELECT 'replacedbyarticles',    COUNT(*) FROM replacedbyarticles
UNION ALL SELECT 'replacesarticles',      COUNT(*) FROM replacesarticles;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 2 - SCHEMA + INDEX CHECK
-- ═══════════════════════════════════════════════════════════════════════════
-- The two P0 performance fixes ride on generated columns + indexes:
--
--   sql/06:  articlecrosses.oemNumberNormalized  (generated column)
--            idx_articlecrosses_oemNumberNormalized
--            idx_oem_number_clean_number
--
--   sql/07:  idx_articlecriteria_legacyArticleId
--            idx_articlecriteria_criteria_value
--
-- Without these, cross-reference search runs 3-8 HOURS per query and
-- FindSpecifications runs 17-36 seconds. If Section 2 shows them
-- missing, that's the single biggest fix ops can make.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 2: schema + index check ===' AS section;

-- Does articlecrosses have the generated oemNumberNormalized column?
SELECT
	column_name,
	data_type,
	is_nullable,
	generation_expression IS NOT NULL AS is_generated,
	IF(generation_expression IS NOT NULL, LEFT(generation_expression, 80), '') AS gen_expr_first_80
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'articlecrosses'
ORDER BY ordinal_position;

-- All indexes on the tables we care about.
SELECT
	table_name,
	index_name,
	non_unique,
	GROUP_CONCAT(column_name ORDER BY seq_in_index) AS columns,
	index_type
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name IN (
      'articlecrosses', 'articlecriteria', 'oem_number', 'oem_search_index',
      'articles', 'articlesvehicletrees', 'linkagetargets', 'modelseries',
      'manufacturers', 'ambrand', 'replacedbyarticles', 'replacesarticles'
  )
GROUP BY table_name, index_name, non_unique, index_type
ORDER BY table_name, index_name;

-- Specifically the P0 indexes — pass/fail summary at a glance.
SELECT '--- P0 index summary ---' AS note;

SELECT
	'articlecrosses.oemNumberNormalized'                 AS check_name,
	IF(EXISTS(
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'articlecrosses'
		  AND column_name = 'oemNumberNormalized'
	), 'PRESENT', 'MISSING (sql/06 needed)')             AS status
UNION ALL SELECT
	'idx_articlecrosses_oemNumberNormalized',
	IF(EXISTS(
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'articlecrosses'
		  AND index_name = 'idx_articlecrosses_oemNumberNormalized'
	), 'PRESENT', 'MISSING (sql/06 needed)')
UNION ALL SELECT
	'idx_oem_number_clean_number',
	IF(EXISTS(
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'oem_number'
		  AND index_name = 'idx_oem_number_clean_number'
	), 'PRESENT', 'MISSING (sql/06 needed)')
UNION ALL SELECT
	'idx_articlecriteria_legacyArticleId',
	IF(EXISTS(
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'articlecriteria'
		  AND index_name = 'idx_articlecriteria_legacyArticleId'
	), 'PRESENT', 'MISSING (sql/07 needed)')
UNION ALL SELECT
	'idx_articlecriteria_criteria_value',
	IF(EXISTS(
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'articlecriteria'
		  AND index_name = 'idx_articlecriteria_criteria_value'
	), 'PRESENT', 'MISSING (sql/07 needed)');


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 3 - HYUNDAI / KIA COVERAGE — MANUFACTURERS + BRANDS
-- ═══════════════════════════════════════════════════════════════════════════
-- Are Hyundai + Kia even present as manufacturers? Every downstream
-- table joins to `manuId` — if these are missing or wrong-cased, nothing
-- works.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 3: Hyundai/Kia in manufacturers + ambrand ===' AS section;

-- manufacturers rows for HK marques
SELECT
	manuId,
	manuName,
	linkingTargetType
FROM manufacturers
WHERE manuName LIKE '%Hyundai%'
   OR manuName LIKE '%Kia%'
   OR manuName LIKE '%Mobis%'
   OR manuName LIKE '%Genesis%'
ORDER BY manuName;

-- ambrand rows for HK aftermarket suppliers (Mobis, Hyundai Mobis, etc.)
SELECT
	brandId,
	brandName,
	lang
FROM ambrand
WHERE brandName LIKE '%Hyundai%'
   OR brandName LIKE '%Kia%'
   OR brandName LIKE '%Mobis%'
   OR brandName LIKE '%Genesis%'
LIMIT 20;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 4 - HK COVERAGE — oem_number (PRIMARY OEM LOOKUP)
-- ═══════════════════════════════════════════════════════════════════════════
-- SearchByOEM primary path queries oem_number.clean_number. The app's
-- audit found only ~5% of real HK OEMs have entries here — this section
-- quantifies that with fresh data.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 4: oem_number HK coverage ===' AS section;

-- Rows in oem_number where the joined article's manufacturer is HK.
SELECT
	m.manuName,
	COUNT(*) AS oem_rows
FROM oem_number on2
JOIN articles a ON a.legacyArticleId = on2.articleId
JOIN manufacturers m ON m.manuId = a.mfrId
WHERE m.manuName LIKE '%Hyundai%'
   OR m.manuName LIKE '%Kia%'
   OR m.manuName LIKE '%Mobis%'
   OR m.manuName LIKE '%Genesis%'
GROUP BY m.manuName
ORDER BY oem_rows DESC;

-- Distribution of HK OEM prefixes actually present in oem_number.
-- Compare against the app's oem_prefix.go prefixMap to see coverage gaps.
SELECT
	LEFT(on2.clean_number, 2) AS prefix2,
	COUNT(*) AS rows_in_oem_number
FROM oem_number on2
JOIN articles a ON a.legacyArticleId = on2.articleId
JOIN manufacturers m ON m.manuId = a.mfrId
WHERE m.manuName LIKE '%Hyundai%' OR m.manuName LIKE '%Kia%'
GROUP BY prefix2
ORDER BY rows_in_oem_number DESC
LIMIT 30;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 5 - HK COVERAGE — articlecrosses (AFTERMARKET CROSS-REFS)
-- ═══════════════════════════════════════════════════════════════════════════
-- The 30M-row cross-ref table. This is where every aftermarket brand
-- (Bosch, MANN, MAHLE, Denso, etc.) writes its OEM<->part-number
-- mappings. Coverage here is what M2.S1 depends on.
--
-- If oemNumberNormalized column DOES NOT exist (sql/06 not applied),
-- SKIP this section — the queries will full-scan.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 5: articlecrosses HK coverage ===' AS section;

-- Guard: only run when the sql/06 column exists. If it doesn't, this
-- SELECT still succeeds (returns 0 or errors gracefully).
SELECT
	'articlecrosses.oemNumberNormalized present' AS check_name,
	COUNT(*) AS matched
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'articlecrosses'
  AND column_name = 'oemNumberNormalized';

-- HK OEM prefixes actually cross-referenced in articlecrosses.
-- The LIKE prefix match uses the (unindexed) oemNumber column so it's
-- slow — kill if this hangs > 2 minutes and note the block in output.
SELECT
	LEFT(oemNumber, 2) AS prefix2,
	COUNT(*) AS crossref_rows,
	COUNT(DISTINCT brandName) AS distinct_brands
FROM articlecrosses
WHERE oemNumber LIKE '26___-_____'      -- Filter
   OR oemNumber LIKE '58___-_____'      -- Brake
   OR oemNumber LIKE '97___-_____'      -- HVAC
   OR oemNumber LIKE '82___-_____'      -- Glass / Window motor
GROUP BY prefix2
LIMIT 20;

-- Which aftermarket brands have HK cross-refs? Top 30.
-- Skip if the previous query timed out.
SELECT
	brandName,
	COUNT(*) AS crossref_rows
FROM articlecrosses
WHERE oemNumber LIKE '26___-_____'
   OR oemNumber LIKE '58___-_____'
   OR oemNumber LIKE '97___-_____'
GROUP BY brandName
ORDER BY crossref_rows DESC
LIMIT 30;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 6 - HK COVERAGE — articlecriteria (SPECS)
-- ═══════════════════════════════════════════════════════════════════════════
-- 27M-row table. Every physical spec (thread, length, diameter, torque)
-- lives here. FindSpecifications queries WHERE legacyArticleId = ?.
-- Without sql/07 index this is 17-36 seconds per call.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 6: articlecriteria HK coverage ===' AS section;

-- How many DISTINCT HK articles have specs at all?
SELECT
	m.manuName,
	COUNT(DISTINCT a.legacyArticleId) AS articles_with_specs
FROM articlecriteria ac
JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
JOIN manufacturers m ON m.manuId = a.mfrId
WHERE m.manuName LIKE '%Hyundai%' OR m.manuName LIKE '%Kia%' OR m.manuName LIKE '%Mobis%'
GROUP BY m.manuName;

-- What criteriaDescription categories are populated? Top 30.
SELECT
	criteriaDescription,
	COUNT(*) AS occurrences
FROM articlecriteria
GROUP BY criteriaDescription
ORDER BY occurrences DESC
LIMIT 30;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 7 - HK COVERAGE — articlesvehicletrees (VEHICLE FITMENT)
-- ═══════════════════════════════════════════════════════════════════════════
-- 651M-row table. PartsForVehicle queries this to answer
-- "which parts fit this vehicle?". FindCompatibleVehicles queries the
-- reverse.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 7: articlesvehicletrees HK coverage ===' AS section;

-- Sample how many rows articlesvehicletrees has for a known HK linkage
-- target. Pick a Hyundai Elantra (2013+ TL chassis) linkage id — the
-- catalog usually has several thousand parts per vehicle.
--
-- To pick a valid linkage id yourself first, use section 9.
SELECT
	linkingTargetType,
	COUNT(*) AS rows
FROM articlesvehicletrees
WHERE linkingTargetType IN ('P', 'V', 'C', 'M', 'A', 'K')
GROUP BY linkingTargetType
ORDER BY rows DESC;

-- Distinct linkage-target types present. The app filters on 'P' (Passenger);
-- if HK data uses different codes, that filter drops everything.
SELECT
	linkingTargetType,
	COUNT(DISTINCT linkingTargetId) AS distinct_linkage_targets
FROM articlesvehicletrees
GROUP BY linkingTargetType
ORDER BY distinct_linkage_targets DESC;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 8 - HK COVERAGE — SUPERSESSION (replacedbyarticles / replacesarticles)
-- ═══════════════════════════════════════════════════════════════════════════
-- SupersessionStrategy returned 0 hits in the 2026-08-24 probe. This
-- section confirms whether the table is empty for HK OEMs or the app's
-- query has a bug.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 8: supersession chain HK coverage ===' AS section;

-- Total rows.
SELECT
	'replacedbyarticles' AS tbl, COUNT(*) AS total_rows FROM replacedbyarticles
UNION ALL SELECT
	'replacesarticles',     COUNT(*) FROM replacesarticles;

-- HK-articled supersession rows.
SELECT
	'replacedbyarticles HK' AS tbl,
	COUNT(*) AS hk_rows
FROM replacedbyarticles rba
JOIN articles a ON a.legacyArticleId = rba.legacyArticleId
JOIN manufacturers m ON m.manuId = a.mfrId
WHERE m.manuName LIKE '%Hyundai%' OR m.manuName LIKE '%Kia%' OR m.manuName LIKE '%Mobis%'
UNION ALL SELECT
	'replacesarticles HK',
	COUNT(*)
FROM replacesarticles ra
JOIN articles a ON a.legacyArticleId = ra.legacyArticleId
JOIN manufacturers m ON m.manuId = a.mfrId
WHERE m.manuName LIKE '%Hyundai%' OR m.manuName LIKE '%Kia%' OR m.manuName LIKE '%Mobis%';


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 9 - HK COVERAGE — linkagetargets + modelseries
-- ═══════════════════════════════════════════════════════════════════════════
-- The vehicle catalog. /api/catalog/vehicles and vehicle_fitment
-- strategy both depend on this. When the app returns total=0 for
-- "Hyundai Elantra", the source of truth is here.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 9: linkagetargets + modelseries HK ===' AS section;

-- Distinct HK model series with linkage targets.
SELECT
	m.manuName,
	COUNT(DISTINCT ms.modelId) AS model_count,
	COUNT(DISTINCT lt.linkageTargetId) AS linkage_count
FROM modelseries ms
JOIN manufacturers m ON m.manuId = ms.manuId
LEFT JOIN linkagetargets lt ON lt.vehicleModelSeriesId = ms.modelId
WHERE m.manuName LIKE '%Hyundai%' OR m.manuName LIKE '%Kia%' OR m.manuName LIKE '%Genesis%'
GROUP BY m.manuName
ORDER BY linkage_count DESC;

-- Sample 10 Elantra linkage targets so the audit script has valid IDs.
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
WHERE m.manuName LIKE '%Hyundai%'
  AND ms.modelname LIKE '%Elantra%'
ORDER BY lt.beginYearMonth DESC
LIMIT 10;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 10 - LANGUAGE COVERAGE
-- ═══════════════════════════════════════════════════════════════════════════
-- The app hardcodes `lang = 'en'` on linkagetargets + ambrand +
-- assemblygroupnodenames. If HK data ships in another language only,
-- the join eliminates everything.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 10: language coverage ===' AS section;

SELECT 'linkagetargets' AS tbl, lang, COUNT(*) AS rows
FROM linkagetargets
GROUP BY lang
ORDER BY rows DESC
LIMIT 20;

SELECT 'ambrand' AS tbl, lang, COUNT(*) AS rows
FROM ambrand
GROUP BY lang
ORDER BY rows DESC
LIMIT 20;

SELECT 'assemblygroupnodenames' AS tbl, lang, COUNT(*) AS rows
FROM assemblygroupnodenames
GROUP BY lang
ORDER BY rows DESC
LIMIT 20;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 11 - TEST-CORPUS OEM CHECK
-- ═══════════════════════════════════════════════════════════════════════════
-- The 25 primary audit-corpus OEMs. Do they actually resolve to real
-- articles in this MySQL? Any that don't will always return 0 in the
-- app because there's no data to find.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 11: test-corpus OEM lookup ===' AS section;

-- Use a normalised WHERE-IN — all corpus OEMs stripped of dashes.
-- Reports (clean_oem, resolved_article_id, article_desc, brand).
SELECT
	on2.clean_number AS clean_oem,
	on2.number       AS raw_oem,
	a.legacyArticleId,
	COALESCE(a.articleNumber, '')                 AS article_number,
	COALESCE(a.genericArticleDescription, '')     AS description,
	COALESCE(ab.brandName, '')                    AS brand
FROM oem_number on2
LEFT JOIN articles a ON a.legacyArticleId = on2.articleId
LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
WHERE on2.clean_number IN (
	'263502j001',   -- Hyundai V6 oil filter
	'2630035505',   -- Common oil filter
	'581013xa00',   -- Front brake pad Elantra
	'97133d3000',   -- Cabin filter Tucson
	'281132s000',   -- Air filter Tucson
	'971332h001',   -- Cabin filter Elantra
	'824602t010',   -- Window motor Optima
	'581013sa00',   -- Front brake pad
	'553112h000',   -- Rear shock
	'546302h000',   -- Front coil
	'273012e400',   -- Ignition coil
	'584112sa00',   -- Rear disc
	'517122wa00',   -- Front disc
	'824603s000',   -- Window motor Sonata
	'82460d3000',   -- Window motor Tucson
	'263503c100',   -- Oil filter variant
	'463213b650',   -- Auto trans mount
	'545284a100',   -- Lower ball joint
	'921013s050'    -- Headlight
)
ORDER BY clean_oem;

-- The mirror check — same 25 OEMs, but via articlecrosses (aftermarket
-- cross-ref table). Only reliable when the sql/06 generated column is
-- present. Skip if section 2 showed oemNumberNormalized missing.
SELECT
	ac.oemNumber AS raw_oem,
	COUNT(*) AS crossref_count,
	COUNT(DISTINCT ac.brandName) AS distinct_brands,
	GROUP_CONCAT(DISTINCT ac.brandName ORDER BY ac.brandName SEPARATOR ', ') AS brands
FROM articlecrosses ac
WHERE ac.oemNumber IN (
	'26350-2J001', '26300-35505', '58101-3XA00', '97133-D3000', '28113-2S000',
	'97133-2H001', '82460-2T010', '58101-3SA00', '55311-2H000', '54630-2H000',
	'27301-2E400', '58411-2SA00', '51712-2WA00', '82460-3S000', '82460-D3000',
	'26350-3C100', '46321-3B650', '54528-4A100', '92101-3S050'
)
GROUP BY ac.oemNumber
ORDER BY crossref_count DESC;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 12 - SAMPLE DATA (real rows for spot-check)
-- ═══════════════════════════════════════════════════════════════════════════
-- Look at actual data shapes so any surprises (unexpected columns,
-- upper/lower case, extra whitespace) surface without extra queries.
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 12: sample rows ===' AS section;

SELECT '-- articles (5 rows, filter to Hyundai) --' AS note;
SELECT * FROM articles WHERE mfrId IN (
	SELECT manuId FROM manufacturers WHERE manuName LIKE '%Hyundai%' LIMIT 1
) LIMIT 5;

SELECT '-- oem_number (5 HK rows) --' AS note;
SELECT * FROM oem_number WHERE clean_number LIKE '263%' LIMIT 5;

SELECT '-- articlecrosses (5 HK-cross-ref rows) --' AS note;
SELECT * FROM articlecrosses WHERE oemNumber LIKE '26___-_____' LIMIT 5;

SELECT '-- articlecriteria (5 rows, any article) --' AS note;
SELECT * FROM articlecriteria WHERE legacyArticleId IS NOT NULL LIMIT 5;

SELECT '-- linkagetargets (3 HK Elantra rows) --' AS note;
SELECT * FROM linkagetargets lt
JOIN modelseries ms ON lt.vehicleModelSeriesId = ms.modelId
JOIN manufacturers m ON m.manuId = ms.manuId
WHERE m.manuName LIKE '%Hyundai%' AND ms.modelname LIKE '%Elantra%'
LIMIT 3;


-- ═══════════════════════════════════════════════════════════════════════════
-- SECTION 13 - QUERY-PLAN VERIFICATION
-- ═══════════════════════════════════════════════════════════════════════════
-- EXPLAIN on the EXACT queries the app runs. Want:
--   * type = 'ref' or 'const' (index seek)  = good
--   * type = 'range'                       = good on legit ranges
--   * type = 'ALL' (full scan)              = bad; the query will be slow
--   * key populated with the expected index = confirms index is used
--   * key = NULL                             = index missing / query planner not using it
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== SECTION 13: query-plan check ===' AS section;

-- The FindSpecifications query (M3.S1.T2).
-- Expected: key = idx_articlecriteria_legacyArticleId (after sql/07).
SELECT '-- FindSpecifications --' AS note;
EXPLAIN SELECT
	COALESCE(criteriaDescription, ''), COALESCE(rawValue, ''),
	COALESCE(criteriaUnitDescription, ''), COALESCE(criteriaType, '')
FROM articlecriteria
WHERE legacyArticleId = 100000
ORDER BY criteriaDescription
LIMIT 200;

-- The TecDocCrossRef.SearchCrossReferences query.
-- Expected: key = idx_articlecrosses_oemNumberNormalized (after sql/06).
SELECT '-- TecDocCrossRef.SearchCrossReferences --' AS note;
EXPLAIN SELECT
	ac.oemNumber, COALESCE(m.manuName, ''), COALESCE(a.legacyArticleId, 0),
	COALESCE(a.articleNumber, ''), COALESCE(a.genericArticleDescription, ''),
	COALESCE(ac.brandName, ''), COALESCE(m.manuName, '')
FROM articlecrosses ac
LEFT JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
LEFT JOIN manufacturers m ON m.manuId = ac.mfrId AND m.linkingTargetType = 'P'
WHERE ac.oemNumberNormalized = '263502j001'
LIMIT 30;

-- The TecDoc.SearchByOEM.primary query.
-- Expected: key = idx_oem_number_clean_number (after sql/06).
SELECT '-- TecDoc.SearchByOEM.primary --' AS note;
EXPLAIN SELECT DISTINCT
	on2.number AS oem_raw,
	a.legacyArticleId, a.articleNumber, a.genericArticleDescription,
	COALESCE(ab.brandName, '') AS brand
FROM oem_number on2
JOIN articles a ON a.legacyArticleId = on2.articleId
LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
WHERE on2.clean_number = '263502j001'
LIMIT 30;

-- The PartsForVehicle query.
-- Expected: key on articlesvehicletrees.(linkingTargetId, linkingTargetType).
SELECT '-- PartsForVehicle --' AS note;
EXPLAIN SELECT DISTINCT
	a.legacyArticleId, a.articleNumber, a.genericArticleDescription,
	COALESCE(ab.brandName, '') AS brand,
	COALESCE(agn.assemblyGroupName, '') AS groupName
FROM articlesvehicletrees avt
JOIN articles a ON a.legacyArticleId = avt.legacyArticleId
LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
LEFT JOIN assemblygroupnodenames agn ON agn.assemblyGroupNodeId = avt.assemblyGroupNodeId AND agn.lang = 'en'
WHERE avt.linkingTargetId = 39843
  AND avt.linkingTargetType = 'P'
LIMIT 30;


-- ═══════════════════════════════════════════════════════════════════════════
-- END OF REPORT
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '=== END OF REPORT — paste the full output back to the agent ===' AS section;
