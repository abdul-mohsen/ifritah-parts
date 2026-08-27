-- ============================================================================
-- TecDoc Audit + Diagnostic — OPTIMIZED
-- ============================================================================
-- Only queries that are still unresolved after the 2026-08-25 / 26 runs.
-- Every query that had a successful answer on prior runs has been removed
-- from this file — those answers are pinned in the local (non-repo) note:
--
--   C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\tecdoc-known-answers\ANSWERED.md
--
-- If the operator ever swaps the TecDoc dump for a different one, re-run
-- the full historical file (in git history under commit 5069e06:
-- `tecdoc_diagnostic_full.sql` — 23 sections) to re-establish the baseline.
--
-- ─── What's IN this file ────────────────────────────────────────────
--
--   Part A · sql/08 apply verification (still flips PRESENT after apply)
--   Part B · vehicle-catalog + supersession + language (unresolved)
--   Part C · THE aftermarket answer (via dataSupplierId — the whole point)
--   Part D · 19-OEM corpus verification (never run)
--   Part F · EXPLAIN plans (validates sql/06+sql/07+sql/08 index-hit)
--
-- ─── What's OUT (already answered — in the local ANSWERED.md) ───────
--
--   Table sizes, exact row counts, index inventory, articlecrosses column
--   list, articles/ambrand/articlecriteria schemas, HK manuIds, ambrand
--   brand catalog, oem_number HK prefix distribution, articlecrosses HK
--   prefix distribution, articlecriteria global spec distribution,
--   articlesvehicletrees linkingTargetType distribution.
--
-- ─── Runtime ─────────────────────────────────────────────────────────
--
--   Estimated 2-4 minutes (was 2-8 min for the full file). Every query
--   uses indexed equality or bounded LIMIT — no COUNT(DISTINCT) over
--   340M rows, no LIMIT-inside-IN-SELECT that materializes huge lists.
--
-- ─── Usage ───────────────────────────────────────────────────────────
--
--   mysql --host=<tecdoc-mysql-host> --user=<user> --password \
--         --database=<tecdoc-db-name> \
--         < scripts/diagnostics/tecdoc_diagnostic_full.sql \
--         > tecdoc-diagnostic-$(date +%Y-%m-%d).txt
--
-- MariaDB 10.3+ AND MySQL 5.7 / 8.x compatible.
-- ============================================================================


SELECT '' AS ' ';
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '  TECDOC DIAGNOSTIC — OPTIMIZED (unresolved queries only)' AS ' ';
SELECT '  Run at:' AS ' ', NOW() AS run_at;
SELECT '  Database:' AS ' ', DATABASE() AS db;
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '' AS ' ';


-- ═══════════════════════════════════════════════════════════════════════════
--         PART A — P0 index PASS/FAIL (re-runnable — flips after sql/08)
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '─── §A1 P0 index PASS/FAIL summary ───────────────────' AS section;

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


-- ═══════════════════════════════════════════════════════════════════════════
--         PART B — Unresolved coverage: vehicle-catalog / supersession /
--                  language (never answered on prior runs)
-- ═══════════════════════════════════════════════════════════════════════════

-- ─── §B1 articlesvehicletrees sampled distinct-linkage stats ─────────
--
-- The full COUNT(DISTINCT linkingTargetId) over 340M rows blew /var/tmp
-- on the v1 followup run. Bounded 10M-row sample keeps temp <40 MB.

SELECT '─── §B1 Sampled distinct P linkages (10M cap) ────────' AS section;

SELECT
	COUNT(DISTINCT sample_id) AS distinct_p_linkages_in_10m_sample
FROM (
	SELECT linkingTargetId AS sample_id
	FROM articlesvehicletrees
	WHERE linkingTargetType = 'P'
	LIMIT 10000000
) t;

SELECT '─── §B1b Rows-per-P-linkage sample (1000 linkages) ───' AS section;

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


-- ─── §B2 Supersession chain HK coverage (memory-safe EXISTS) ────────

SELECT '─── §B2 Supersession schemas ─────────────────────────' AS section;

SELECT
	TABLE_NAME,
	COLUMN_NAME,
	DATA_TYPE
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name IN ('replacedbyarticles', 'replacesarticles')
ORDER BY TABLE_NAME, ORDINAL_POSITION;

SELECT '─── §B2b Sample replacedbyarticles rows ──────────────' AS section;
SELECT * FROM replacedbyarticles LIMIT 5;

SELECT '─── §B2c Sample replacesarticles rows ────────────────' AS section;
SELECT * FROM replacesarticles LIMIT 5;

SELECT '─── §B2d HK supersession rows via articlecrosses JOIN ─' AS section;

SELECT
	'replacedbyarticles HK'   AS tbl,
	COUNT(*)                  AS matched_rows,
	COUNT(DISTINCT rba.legacyArticleId) AS distinct_articles
FROM replacedbyarticles rba
WHERE EXISTS (
	SELECT 1 FROM articlecrosses acr
	WHERE acr.legacyArticleId = rba.legacyArticleId
	  AND (
		acr.oemNumberNormalized LIKE '263%' OR acr.oemNumberNormalized LIKE '581%' OR
		acr.oemNumberNormalized LIKE '821%' OR acr.oemNumberNormalized LIKE '971%'
	  )
);

SELECT
	'replacesarticles HK'     AS tbl,
	COUNT(*)                  AS matched_rows,
	COUNT(DISTINCT ra.legacyArticleId) AS distinct_articles
FROM replacesarticles ra
WHERE EXISTS (
	SELECT 1 FROM articlecrosses acr
	WHERE acr.legacyArticleId = ra.legacyArticleId
	  AND (
		acr.oemNumberNormalized LIKE '263%' OR acr.oemNumberNormalized LIKE '581%' OR
		acr.oemNumberNormalized LIKE '821%' OR acr.oemNumberNormalized LIKE '971%'
	  )
);


-- ─── §B3 HK vehicle catalog (uses confirmed manuIds) ─────────────────

SELECT '─── §B3 HK models + linkage counts ───────────────────' AS section;

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

SELECT '─── §B3b Sample Elantra linkage IDs ──────────────────' AS section;

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

SELECT '─── §B3c Sample Sonata linkage IDs ───────────────────' AS section;

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


-- ─── §B4 Language distribution ───────────────────────────────────────

SELECT '─── §B4 linkagetargets language coverage ─────────────' AS section;

SELECT 'linkagetargets' AS tbl, lang, COUNT(*) AS row_count
FROM linkagetargets
GROUP BY lang
ORDER BY row_count DESC
LIMIT 10;

SELECT '─── §B4b ambrand language coverage ───────────────────' AS section;

SELECT 'ambrand' AS tbl, lang, COUNT(*) AS row_count
FROM ambrand
GROUP BY lang
ORDER BY row_count DESC
LIMIT 10;


-- ═══════════════════════════════════════════════════════════════════════════
--         PART C — REAL aftermarket answer (the whole reason we run this)
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Every earlier attempt got misleading answers because articlecrosses.brandName
-- is the CROSS-REFED OEM brand, not the aftermarket-brand. The REAL aftermarket
-- brand lives at:
--
--   articlecrosses.legacyArticleId
--     → articles.dataSupplierId  (NOT articles.mfrId — mfrId is 0 everywhere)
--       → ambrand.brandId  (with ambrand.lang='EN')
--
-- This section does the correct JOIN and gives the definitive answer to
-- "does the TecDoc dump on this DB have Bosch/MANN/MAHLE/etc. for HK OEMs?"

-- ─── §C1 Aftermarket brands per HK prefix (correct JOIN) ────────────

SELECT '─── §C1 REAL aftermarket brands on HK cross-refs ─────' AS section;

SELECT
	amb.brandName                       AS aftermarket_brand,
	COUNT(*)                            AS crossref_rows,
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
ORDER BY crossref_rows DESC
LIMIT 30;

-- ─── §C2 Explicit marquee-brand probe ───────────────────────────────

SELECT '─── §C2 Marquee brands: BOSCH/MANN/MAHLE/etc. on HK ──' AS section;

SELECT
	amb.brandName,
	COUNT(*) AS hk_cross_ref_rows
FROM ambrand amb
JOIN articles       a   ON a.dataSupplierId = amb.brandId
JOIN articlecrosses acr ON acr.legacyArticleId = a.legacyArticleId
WHERE amb.lang = 'EN'
  AND amb.brandName IN (
	'BOSCH','MANN-FILTER','MAHLE','MAHLE ORIGINAL','KNECHT','DENSO',
	'NGK','VALEO','HELLA','BREMBO','TEXTAR','FERODO','SACHS','FEBI',
	'FEBI BILSTEIN','LEMFOERDER','LUK','INA','SKF','GATES',
	'CONTINENTAL','FILTRON','WIX','PURFLUX','MAGNETI MARELLI',
	'DELPHI','MEYLE','TRW','ATE','ZIMMERMANN','BLUE PRINT','KYB',
	'MONROE','SANGSIN','MOBIS','PIERBURG','ELRING','VICTOR REINZ'
  )
  AND (
	acr.oemNumberNormalized LIKE '263%' OR acr.oemNumberNormalized LIKE '581%' OR
	acr.oemNumberNormalized LIKE '821%' OR acr.oemNumberNormalized LIKE '971%'
  )
GROUP BY amb.brandName
ORDER BY hk_cross_ref_rows DESC;


-- ═══════════════════════════════════════════════════════════════════════════
--         PART D — 19-OEM audit corpus verification
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '─── §D1 Loading 19-OEM audit corpus temp table ───────' AS section;

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

SELECT COUNT(*) AS corpus_size FROM tmp_corpus;

-- ─── §D2 Corpus lookup via oem_number ──────────────────────────────

SELECT '─── §D2 Corpus OEMs via oem_number.clean_number ──────' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(on2.id)      AS oem_number_rows,
	MIN(on2.articleId) AS sample_article_id
FROM tmp_corpus c
LEFT JOIN oem_number on2 ON on2.clean_number = c.normalized
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;

-- ─── §D3 Corpus lookup via articlecrosses + brand diversity ────────

SELECT '─── §D3 Corpus crossref counts + brand diversity ─────' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(acr.id)                 AS crossref_rows,
	COUNT(DISTINCT acr.brandName) AS distinct_brands
FROM tmp_corpus c
LEFT JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
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
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
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
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
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
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
JOIN articlecriteria ac ON ac.legacyArticleId = acr.legacyArticleId
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;

DROP TEMPORARY TABLE IF EXISTS tmp_corpus;


-- ═══════════════════════════════════════════════════════════════════════════
--         PART F — EXPLAIN plans (validates every hot query hits an index)
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Every EXPLAIN below should show `type=ref` (or better) with a meaningful
-- `key`. When any of them shows `type=ALL` (full table scan), the
-- corresponding sql/06 / sql/07 / sql/08 migration is missing.
-- Correlate with the §A1 pass/fail summary at the top.

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
SELECT '  DIAGNOSTIC COMPLETE — paste full output back for analysis' AS ' ';
SELECT '  Completed at:' AS ' ', NOW() AS completed_at;
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
